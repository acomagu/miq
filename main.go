package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/julienschmidt/httprouter"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

// Response defines the JSON contents for response to resource request by HTTP.
type Response struct {
	Success          bool                     `json:"success"`
	Rows             []map[string]interface{} `json:"rows"`
	ErrorType        string                   `json:"errorType"`
	ErrorDescription string                   `json:"errorDescription"`
}

// TimeoutError causes when exceed given deadline for retring callback.
type TimeoutError error

func main() {
	inputConfig, err := readConfigs()
	if err != nil {
		fmt.Println(err)
		return
	}

	config, err := inputConfig.Config()
	if err != nil {
		fmt.Printf("invalid configuration: %s\n", err)
		return
	}

	db, err := openDB(config)
	if err != nil {
		fmt.Println(err)
		return
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

func openDB(config Config) (*sqlx.DB, error) {
	dsn, err := config.DB.DSN()
	if err != nil {
		return nil, err
	}

	db, err := sqlx.Open(config.DB.Driver, dsn)
	if err != nil {
		return nil, err
	}
	err = retryDuring(30*time.Second, 1*time.Second, func() error {
		return db.Ping()
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func retryDuring(duration time.Duration, sleep time.Duration, callback func() error) error {
	timeout := time.After(duration)

	for i := 0; ; i++ {
		retry := time.After(sleep)

		err := callback()
		if err == nil {
			return nil
		}

		select {
		case <-timeout:
			return TimeoutError(fmt.Errorf("after %d attempts (during %s), last error: %s", i, duration, err))
		case <-retry:
		}
	}
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
