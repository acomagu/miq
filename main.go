package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"

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

// InputRule is part of InputConfig
type InputRule struct {
	Path        string
	Before      string
	Befores     []string
	Query       string
	Queries     []string
	After       string
	Afters      []string
	Method      string
	Transaction bool
}

// DBConfig is parameters for opening connection to database.
type DBConfig struct {
	Driver   string `yaml:"driver"`
	Filepath string `yaml:"filepath"`
}

// InputConfig is for reading YAML config file.
type InputConfig struct {
	DB    DBConfig    `yaml:"db"`
	Rules []InputRule `yaml:"rules"`
	Port  int         `yaml:"port"`
}

// QuerySet contains all queries to execute for a Rule.
type QuerySet struct {
	Befores     []Query
	Queries     []Query
	Afters      []Query
	Transaction bool
}

func newQuerySet(db *sqlx.DB, rule Rule) (QuerySet, error) {
	befores, err := newQueries(db, rule.Befores)
	if err != nil {
		return QuerySet{}, err
	}

	queries, err := newQueries(db, rule.Queries)
	if err != nil {
		return QuerySet{}, err
	}

	afters, err := newQueries(db, rule.Afters)
	if err != nil {
		return QuerySet{}, err
	}

	return QuerySet{
		Befores:     befores,
		Queries:     queries,
		Afters:      afters,
		Transaction: rule.Transaction,
	}, nil
}

func newQueries(db *sqlx.DB, qss []string) ([]Query, error) {
	queries := []Query{}
	for _, qs := range qss {
		q, err := newQuery(db, qs)
		if err != nil {
			return []Query{}, err
		}
		queries = append(queries, q)
	}
	return queries, nil
}

func newQuery(db *sqlx.DB, qs string) (Query, error) {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	groups := re.FindAllStringSubmatch(qs, -1)

	argKeys := []string{}
	for _, group := range groups {
		argKeys = append(argKeys, group[1])
	}
	replaced := re.ReplaceAllString(qs, "?")
	stmt, err := db.Preparex(replaced)
	if err != nil {
		return Query{}, SQLParseError(errors.Wrap(err, "failed to parse SQL"))
	}
	return Query{
		SQL:     stmt,
		ArgKeys: argKeys,
	}, nil
}

// Rule is part of `Config`, it's set of the routing path and the SQL query.
type Rule struct {
	Befores     []string
	Queries     []string
	Afters      []string
	Path        string
	Method      Method
	Transaction bool
}

// Config contains configs of whole app
type Config struct {
	DB    DBConfig
	Rules []Rule
	Port  int
}

// Config returns one converted to Config struct
func (ic InputConfig) Config() (Config, error) {
	rules, err := normalizeRules(ic.Rules)
	if err != nil {
		return Config{}, err
	}

	port, err := normalizePort(ic.Port)
	if err != nil {
		return Config{}, err
	}

	return Config{
		DB:    ic.DB,
		Rules: rules,
		Port:  port,
	}, nil
}

func normalizePort(orig int) (int, error) {
	if orig == 0 {
		return 80, nil
	}
	return orig, nil
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

	transaction, err := normalizeTransaction(orig.Transaction)
	if err != nil {
		return Rule{}, err
	}

	return Rule{
		Befores:     befores,
		Queries:     queries,
		Afters:      afters,
		Method:      method,
		Path:        path,
		Transaction: transaction,
	}, nil
}

func normalizeTransaction(orig bool) (bool, error) {
	return orig, nil
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

// UnknownArgError causes when given undefined parameters name in path at SOL.
type UnknownArgError error

// SQLParseError causes when failed to parse SQL.
type SQLParseError error

// Query is data set for one SQL query execution
type Query struct {
	SQL     *sqlx.Stmt
	ArgKeys []string
}

// ExecuteWithArgMap executes the Query. `params` is map of param key and value.
func (q Query) ExecuteWithArgMap(params map[string]interface{}) (*sqlx.Rows, error) {
	args := []interface{}{}
	for _, argKey := range q.ArgKeys {
		arg, ok := params[argKey]
		if !ok {
			return nil, UnknownArgError(errors.Errorf("unknown argument name: %s", argKey))
		}
		args = append(args, arg)
	}
	return q.SQL.Queryx(args...)
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
		w.Header().Set("Content-Type", "application/json")
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

func executeQuerySet(db *sqlx.DB, querySet QuerySet, paramMap map[string]interface{}) ([]map[string]interface{}, error) {
	var op sqlx.Queryer
	if querySet.Transaction {
		var err error
		op, err = db.Beginx()
		if err != nil {
			return nil, err
		}
	} else {
		op = db
	}

	err := executeQueries(op, querySet.Befores, paramMap)
	if err != nil {
		_ = rollbackPossibly(op)
		return nil, err
	}

	rowMaps, err := executeQueriesRowMaps(op, querySet.Queries, paramMap)
	if err != nil {
		_ = rollbackPossibly(op)
		return nil, err
	}

	err = executeQueries(op, querySet.Afters, paramMap)
	if err != nil {
		_ = rollbackPossibly(op)
		return nil, err
	}
	_ = commitPossibly(op)
	return rowMaps, nil
}

func rollbackPossibly(op sqlx.Queryer) bool {
	if tx, ok := op.(*sqlx.Tx); ok {
		tx.Rollback()
		return true
	}
	return false
}

func commitPossibly(op sqlx.Queryer) bool {
	if tx, ok := op.(*sqlx.Tx); ok {
		tx.Commit()
		return true
	}
	return false
}

func getErrorType(err error) string {
	switch err.(type) {
	case QueryExecutionError:
		return "QueryExecutionError"
	}
	return "Unknown"
}

func executeQueries(db sqlx.Queryer, qs []Query, paramMap map[string]interface{}) error {
	_, err := executeQueriesRowMaps(db, qs, paramMap)
	return err
}

func executeQueriesRowMaps(db sqlx.Queryer, qs []Query, paramMap map[string]interface{}) ([]map[string]interface{}, error) {
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

func executeRowMaps(db sqlx.Queryer, q Query, paramMap map[string]interface{}) ([]map[string]interface{}, error) {
	rows, err := q.ExecuteWithArgMap(paramMap)
	if err != nil {
		return nil, QueryExecutionError(errors.Wrap(err, "failed to execute query"))
	}
	res, err := createMapSliceFromRows(rows)
	if err != nil {
		return nil, QueryExecutionError(errors.Wrap(err, "failed to create map slice from results"))
	}
	return res, nil
}

// RequestBodyParseError causes when invalid JSON given as request body.
type RequestBodyParseError error

func createParamMap(req *http.Request, params httprouter.Params) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for _, param := range params {
		result[param.Key] = param.Value
	}

	reqBody := make(map[string]interface{})
	err := json.NewDecoder(req.Body).Decode(&reqBody)
	defer req.Body.Close()
	if err != io.EOF {
		if err != nil {
			return nil, RequestBodyParseError(errors.Wrap(err, "request body must be only 1 hieralchical key/value pairs"))
		}
		for k, v := range reqBody {
			result[k] = v
		}
	}

	// Note that only string value can be passed by URL Queries.
	q := req.URL.Query()
	for k, v := range q {
		result[k] = v[0]
	}

	return result, nil
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
