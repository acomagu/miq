package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/julienschmidt/httprouter"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Response defines the JSON contents for response to resource request by HTTP.
type Response struct {
	Success          bool                     `json:"success"`
	Rows             []map[string]interface{} `json:"rows"`
	ErrorType        string                   `json:"errorType"`
	ErrorDescription string                   `json:"errorDescription"`
}

func main() {
	inputConfig := InputConfig{}
	bts, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		return
	}
	yaml.Unmarshal(bts, &inputConfig)
	config, err := inputConfig.Config()
	if err != nil {
		fmt.Printf("invalid configuration: %s\n", err)
		return
	}

	db, err := sqlx.Open(config.DB.Driver, config.DB.Filepath)
	if err != nil {
		panic(err)
	}

	router := httprouter.New()
	for _, rule := range config.Rules {
		querySet, err := newQuerySet(db, rule)
		if err != nil {
			fmt.Println(errors.Wrap(err, "failed to compile query"))
			return
		}
		router.Handle(string(rule.Method), rule.Path, createHandler(db, querySet))
	}
	fmt.Println(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), router))
}

func createHandler(db *sqlx.DB, q QuerySet) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		paramMap, err := createParamMap(req, params)
		if err != nil {
			w.Write(createErrorResponseBytes(err))
			return
		}
		rowMaps, err := executeQuerySet(db, q, paramMap)
		if err != nil {
			w.Write(createErrorResponseBytes(err))
			return
		}

		bts, marshalErr := json.Marshal(Response{
			Success: true,
			Rows:    rowMaps,
		})
		if marshalErr != nil {
			panic(marshalErr)
		}
		w.Write(bts)
	}
}

func createErrorResponseBytes(err error) []byte {
	bts, marshalErr := json.Marshal(Response{
		Success:          false,
		ErrorType:        getErrorType(err),
		ErrorDescription: err.Error(),
	})
	if marshalErr != nil {
		panic(marshalErr)
	}
	return bts
}
