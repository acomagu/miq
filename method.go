package main

import (
	"fmt"
)

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
