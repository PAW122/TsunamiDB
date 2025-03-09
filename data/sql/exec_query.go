package sql

import (
	sql_handlers "TsunamiDB/data/sql/handlers"
	"TsunamiDB/types"
)

// Status: 0 - ok
type Save_res struct {
	Status int8
	Error  error
}

func Execute_Sql(query types.SQL_req) Save_res {

	switch query.Query {
	case "create_table":
		err := sql_handlers.CreateTable(query)
		return Save_res{Status: 0, Error: err}
	}

	return Save_res{Status: 1}
}
