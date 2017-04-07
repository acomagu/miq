package main

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"github.com/jmoiron/sqlx"
	"github.com/julienschmidt/httprouter"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

// UnknownArgError causes when given undefined parameters name in path at SOL.
type UnknownArgError error

// QueryExecutionError causes in execution SQL query
type QueryExecutionError error

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
