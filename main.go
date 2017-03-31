package main

import (
	"fmt"
	"net/http"
	"os"
	"encoding/json"
	"gopkg.in/yaml.v2"
	"github.com/jmoiron/sqlx"
	"github.com/julienschmidt/httprouter"
	"text/template"
	_ "github.com/mattn/go-sqlite3"
)

var settings = `
driver: sqlite3
rules:
  - path: /show/:id
    query: SELECT * FROM test WHERE id = '{{.id}}';
  - path: /create/
    query: INSERT INTO test (body) VALUES ("lililil"), ("OUE");
`

// Response defines the JSON contents for response to resource request by HTTP.
type Response struct {
	Success bool `json:"success"`
	Rows []map[string]interface{} `json:"rows"`
	ErrorType string `json:"errorType"`
	ErrorDescription string `json:"errorDescription"`
}

// InputRule is part of InputConfig
type InputRule struct {
	Path string
	Query string
	Queries []string
	Method string
}

// InputConfig is for reading YAML config file
type InputConfig struct {
	Driver string `yaml:"driver"`
	Rules []InputRule `yaml:"rules"`
}

// Rule is part of `Config`, it's set of the routing path and the SQL query.
type Rule struct {
	Path string
	Queries []string
	Method Method
}

// Config contains configs of whole app
type Config struct {
	Driver string
	Rules []Rule
}

// Config returns one converted to Config struct
func (ic InputConfig) Config() (Config, error) {
	rules, err := normalizeRules(ic.Rules)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Driver: ic.Driver,
		Rules: rules,
	}, nil
}

func normalizeRules(orig []InputRule) ([]Rule, error) {
	result := []Rule{}
	for _, rule := range orig {
		normalized, err := normalizeRule(rule)
		if err != nil {
			return []Rule{}, err
		}
		result = append(result, normalized)
	}
	return result, nil
}

func normalizeRule(orig InputRule) (Rule, error) {
	queries, err := normalizeQuery(orig.Query, orig.Queries)
	if err != nil {
		return Rule{}, err
	}

	method, err := newMethod(orig.Method)
	if err != nil {
		return Rule{}, err
	}

	path, err := normalizePath(orig.Path)
	if err != nil {
		return Rule{}, err
	}

	return Rule{
		Queries: queries,
		Method: method,
		Path: path,
	}, nil
}

func normalizePath(orig string) (string, error) {
	return orig, nil
}

func normalizeQuery(query string, queries []string) ([]string, error) {
	if query == "" && len(queries) == 0 {
		return []string{}, fmt.Errorf("both of `query` and `queries` can't be defined in a rule")
	} else if query != "" {
		return []string{query}, nil
	} else if len(queries) > 0 {
		return queries, nil
	}
	return []string{}, fmt.Errorf("at least one SQL query must be given per rule")
}

func newMethod(method string) (Method, error) {
	switch method {
	case "GET":
		return GET, nil
	case "POST":
		return POST, nil
	case "PUT":
		return PUT, nil
	case "PATCH":
		return PATCH, nil
	case "DELETE":
		return DELETE, nil
	}
	return "", fmt.Errorf("invalid method name")
}


// Method is enum for HTTP methods.
type Method string

const (
	// GET expresses GET method
	GET Method = "GET"

	// POST expresses POST method
	POST = "POST"

	// PUT expresses PUT method
	PUT = "PUT"

	// PATCH expresses PATCH method
	PATCH = "PATCH"

	// DELETE expresses DELETE method
	DELETE = "DELETE"
)

func main() {
	inputConfig := InputConfig{}
	yaml.Unmarshal([]byte(settings), &inputConfig)
	config, err := inputConfig.Config()
	if err != nil {
		fmt.Printf("invalid configuration: %s\n", err)
		return
	}

	db, err := sqlx.Open(config.Driver, "./test.db")
	if err != nil {
		panic(err)
	}

	router := httprouter.New()
	for _, rule := range config.Rules {
		router.Handle(string(rule.Method), rule.Path, createHandler(db, rule))
	}
	fmt.Println(http.ListenAndServe(":8000", router))
}

func createHandler(db *sqlx.DB, r Rule) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
		t, err := template.New("sql").Parse(r.Queries)
		if err != nil {
			fmt.Println("invalid SQL template")
			return
		}
		err = t.Execute(os.Stdout, createParamMap(params))
		if err != nil {
			fmt.Println(err)
			return
		}

		rows, err := db.Queryx(r.SQL)
		if err != nil {
			bts, err2 := json.Marshal(response{
				Success: false,
				ErrorType: "sqlExecutionError",
				ErrorDescription: err.Error(),
			})
			if err2 != nil {
				panic(err2)
			}
			w.Write(bts)
			return
		}
		res, err := createMapSliceFromRows(rows)
		if err != nil {
			bts, err2 := json.Marshal(response{
				Success: false,
				ErrorType: "sqlExecutionError",
				ErrorDescription: err.Error(),
			})
			if err2 != nil {
				panic(err2)
			}
			w.Write(bts)
			return
		}
		if err != nil {
			panic(err)
		}
		bts, err := json.Marshal(response{
			Success: true,
			Rows: res,
		})
		if err != nil {
			panic(err)
		}
		w.Write(bts)
	}
}

func createParamMap(params httprouter.Params) map[string]string {
	result := make(map[string]string)
	for _, param := range params {
		result[param.Key] = param.Value
	}
	return result
}

type mapString []byte

func (bts mapString) MarshalText() ([]byte, error) {
	return bts, nil
}

func createMapSliceFromRows(rows *sqlx.Rows) ([]map[string]interface{}, error) {
	result := []map[string]interface{}{}
	for rows.Next() {
		cols := make(map[string]interface{})
		err := rows.MapScan(cols)
		if err != nil {
			return nil, err
		}

		newCols := make(map[string]interface{})
		for k, v := range cols {
			if str, ok := v.([]byte); ok {
				newCols[k] = mapString(str)
			} else {
				newCols[k] = v
			}
		}
		result = append(result, newCols)
	}
	return result, nil
}
