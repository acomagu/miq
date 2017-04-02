# miq
Create API server with RDB from few lines of YAML.

## Description
`miq` is simple API server defined by YAML file. It's connected with your favorite RDB, runs SQL queries and returns the result as JSON response.

This approach is inspired by Microservices. This is for only internal, not for public server(Authentification is not supported).

## Usage

```yaml
db:
  driver: sqlite3
  filepath: test.db
rules:
  - path: /show/:id/
    query: SELECT * FROM test WHERE id = {{id}};
  - path: /showall/
    queries:
      - SELECT * FROM test;
    transaction: true
  - path: /create/
    query: INSERT INTO test (body) VALUES ({{body}});
```

Go `/show/3`, and you can get `{"success":true,"rows":[{ ... ,"id":3}]}` or something like this.

To start server, run

```bash
$ miq sample.yml
```

## Requirement
- Go
- sqlite3(other RDB is not supported yet)

## Install
```bash
$ go get github.com/acomagu/miq
```

## Author
[acomagu](https://github.com/acomagu)