package sqlbuilder

import "fmt"

func addAliasToCols(alias string, cols ...string) []string {
	identCols := make([]string, 0, len(cols))

	for _, col := range cols {
		identCols = append(identCols, 
			fmt.Sprintf("%s.%s", alias, col),
		)
	}

	return identCols
}