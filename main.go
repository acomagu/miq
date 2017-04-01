package main

import (
	"fmt"
	"net/http"
	"os"
	"github.com/pkg/errors"
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
	Before string
	Befores []string
	Query string
	Queries []string
	After string
	Afters []string
	Method string
	Transaction bool
}

// InputConfig is for reading YAML config file
type InputConfig struct {
	Driver string `yaml:"driver"`
	Rules []InputRule `yaml:"rules"`
}

// QuerySet contains all queries to execute.
type QuerySet struct {
	Befores []string
	Queries []string
	Afters []string
}

// Rule is part of `Config`, it's set of the routing path and the SQL query.
type Rule struct {
	QuerySet
	Path string
	Method Method
	Transaction bool
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
	befores, err := normalizeBefores(orig.Before, orig.Befores)
	if err != nil {
		return Rule{}, err
	}

	queries, err := normalizeQuery(orig.Query, orig.Queries)
	if err != nil {
		return Rule{}, err
	}

	afters, err := normalizeAfters(orig.After, orig.Afters)
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

	querySet := QuerySet{
		Befores: befores,
		Queries: queries,
		Afters: afters,
	}

	return Rule{
		QuerySet: querySet,
		Method: method,
		Path: path,
	}, nil
}

func normalizePath(orig string) (string, error) {
	return orig, nil
}

func normalizeBefores(before string, befores []string) ([]string, error) {
	if before != "" && len(befores) != 0 {
		return []string{}, fmt.Errorf("both of `before` and `befores` can't be defined in a rule")
	} else if before != "" {
		return []string{before}, nil
	} else if len(befores) > 0 {
		return befores, nil
	}
	return []string{}, nil
}

func normalizeAfters(after string, afters []string) ([]string, error) {
	if after != "" && len(afters) != 0 {
		return []string{}, fmt.Errorf("both of `after` and `afters` can't be defined in a rule")
	} else if after != "" {
		return []string{after}, nil
	} else if len(afters) > 0 {
		return afters, nil
	}
	return []string{}, nil
}

func normalizeQuery(query string, queries []string) ([]string, error) {
	if query != "" && len(queries) != 0 {
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
	case "":
		return GET, nil
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

// QueryExecutionError causes in execution SQL query
type QueryExecutionError error

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
		router.Handle(string(rule.Method), rule.Path, createHandler(db, rule.QuerySet))
	}
	fmt.Println(http.ListenAndServe(":8000", router))
}

func createHandler(db *sqlx.DB, q QuerySet) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
		rowMaps, err := executeQueriesRowMaps(db, q.Queries, createParamMap(params))
		if err != nil {
			bts, marshalErr := json.Marshal(Response{
				Success: false,
				ErrorType: getErrorType(err),
				ErrorDescription: err.Error(),
			})
			if marshalErr != nil {
				panic(marshalErr)
			}
			w.Write(bts)
		}

		bts, marshalErr := json.Marshal(Response{
			Success: true,
			Rows: rowMaps,
		})
		if marshalErr != nil {
			panic(marshalErr)
		}
		w.Write(bts)
	}
}

func getErrorType(err error) string {
	switch err.(type) {
	case QueryExecutionError:
		return "QueryExecutionError"
	}
	return "Unknown"
}

func executeQueriesRowMaps(db *sqlx.DB, qs []string, paramMap map[string]string) ([]map[string]interface{}, error) {
	concated := []map[string]interface{}{}
	for _, q := range qs {
		rowMap, err := executeRowMaps(db, q, paramMap)
		if err != nil {
			return nil, err
		}
		concated = append(concated, rowMap...)
	}
	return concated, nil
}

func executeRowMaps(db *sqlx.DB, q string, paramMap map[string]string) ([]map[string]interface{}, error) {
	t, err := template.New("sql").Parse(q)
	if err != nil {
		return nil, err
	}
	err = t.Execute(os.Stdout, paramMap)
	if err != nil {
		return nil, err
	}

	rows, err := db.Queryx(q)
	if err != nil {
		return nil, QueryExecutionError(errors.Wrap(err, "failed to execute query"))
	}
	res, err := createMapSliceFromRows(rows)
	if err != nil {
		return nil, QueryExecutionError(errors.Wrap(err, "failed to create map slice from results"))
	}
	return res, nil
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
