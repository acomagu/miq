# miq
[![Docker Build Statu](https://img.shields.io/docker/build/acomagu/miq.svg?style=flat-square)](https://hub.docker.com/r/acomagu/miq/) [![CircleCI](https://img.shields.io/circleci/project/github/acomagu/miq.svg?style=flat-square)](https://circleci.com/gh/acomagu/miq) [![Go Report Card](https://goreportcard.com/badge/github.com/acomagu/miq?style=flat-square)](https://goreportcard.com/report/github.com/acomagu/miq)

Create API server with RDB from few lines of YAML.

## Description
`miq` is simple API server defined by YAML file. It's connected with your favorite RDB, runs SQL queries and returns the result as JSON response.

This approach is inspired by Microservices. This is for only internal, not for public server(Authentification is not supported).

## Demo

You can try [Sample with Docker](https://github.com/acomagu/miq-mysql-sample) immediately!

## Usage

```yaml
db:
  driver: sqlite3
  filepath: test.db
rules:
  - path: /show/:id/
    query: SELECT * FROM test WHERE id = {{id}};
  - path: /showall/
    query: SELECT * FROM test;
  - path: /create/
    query: INSERT INTO test (body) VALUES ({{body}});
```

Go `/show/3`, and you can get `{"success":true,"rows":[{ ... ,"id":3}]}` or something like this.

Or you can give variables as URL query or request body(JSON). For example, request `/create?body='abc'` and a record is inserted.

To start server, run

```bash
$ miq sample.yml
```

## Requirement
- Go
- RDB(sqlite3 and mysql are currently supported)

## Install
```bash
$ go get github.com/acomagu/miq
```

## Author
[acomagu](https://github.com/acomagu)
