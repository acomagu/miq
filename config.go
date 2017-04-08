package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/caarlos0/env"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Rule is part of `Config`, it's set of the routing path and the SQL query.
type Rule struct {
	Befores     []string
	Queries     []string
	Afters      []string
	Path        string
	Method      Method
	Transaction bool
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
	Driver   string `yaml:"driver" env:"DB_DRIVER"`
	Filepath string `yaml:"filepath" env:"DB_FILEPATH"`
	Name     string `yaml:"name" env:"DB_NAME"`
	Username string `yaml:"username" env:"DB_USERNAME"`
	Password string `yaml:"password" env:"DB_PASSWORD"`
}

// InputConfig is for reading YAML config file.
type InputConfig struct {
	DB    DBConfig    `yaml:"db"`
	Rules []InputRule `yaml:"rules"`
	Port  int         `yaml:"port"`
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

func readConfigs() (InputConfig, error) {
	inputConfig := InputConfig{}

	// Read configuration from YAML
	bts, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		return InputConfig{}, errors.Wrap(err, "failed to read config file")
	}
	yaml.Unmarshal(bts, inputConfig)

	// Read it from environment variables
	env.Parse(inputConfig)

	return inputConfig, nil
}
