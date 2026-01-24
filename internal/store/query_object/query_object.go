package queryobject

type QueryObject interface {
	ToSql() (string, []any, error)
}
