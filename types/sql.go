package types

type SQL_req struct {
	Query     string   `json:"query,omitempty"`
	TableName string   `json:"tableName,omitempty"`
	Data      any      `json:"data,omitempty"`
	Columns   []Colums `json:"columns,omitempty"`
}

type Colums struct {
	Name         string `json:"name"`         // colmun name
	Type         string `json:"type"`         // data type
	IsPrimaryKey bool   `json:"isPrimaryKey"` // prim key
	Default      string `json:"default"`      // defaultowa wartosc po stworzeniu
	MaxByteSize  int    `json:"maxByteSize"`  // wielkosc miejsca w kolumnie
}
