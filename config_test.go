package main

import (
	"testing"
	"reflect"
)

func TestNormalizeRules(t *testing.T) {
	inputs := []InputRule{
		InputRule{
			Path: "/aaa/",
			Query: "SELECT * FROM test",
			Method: "GET",
		},
	}
	expects := []Rule{
		Rule{
			Befores: []string{},
			Queries: []string{"SELECT * FROM test"},
			Afters: []string{},
			Path: "/aaa/",
			Method: GET,
			Transaction: false,
		},
	}

	results, err := normalizeRules(inputs)
	if err != nil {
		t.Error(err)
		return
	}

	for i, result := range results {
		expect := expects[i]
		if !result.equalTo(expect) {
			t.Errorf("got unexpected result: %+v", result)
		}
	}
}

func (a Rule) equalTo(b Rule) bool {
	return reflect.DeepEqual(a, b)
}

func TestDBConfigDSN(t *testing.T) {
	inputs := []DBConfig{
		DBConfig{
			Driver: "sqlite3",
			Filepath: "./test.db",
		},
		DBConfig{
			Driver: "mysql",
			Name: "app",
			Username: "user",
			Password: "pass",
			Net: "tcp",
			Host: "localhost",
			Port: "3306",
		},
	}
	expects := []string{
		"./test.db",
		"user:pass@tcp(localhost:3306)/app",
	}

	for i, input := range inputs {
		expect := expects[i]
		result, err := input.DSN()
		if err != nil {
			t.Error(err)
		}
		if result != expect {
			t.Errorf("got unexpected result: %s", result)
		}
	}
}
