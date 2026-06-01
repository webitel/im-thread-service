package postgres

import "github.com/jackc/pgx/v5"

func collectRows[ResultType, FlatType any](rows pgx.Rows, flatToResultTypeMapper func(*FlatType) (*ResultType, error)) ([]*ResultType, error) {
	scannedFlatTypes, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[FlatType])
	if err != nil {
		return nil, err
	}

	res := make([]*ResultType, 0, len(scannedFlatTypes))

	for _, member := range scannedFlatTypes {
		domainModel, err := flatToResultTypeMapper(member)
		if err != nil {
			return nil, err
		}

		res = append(res, domainModel)
	}

	return res, nil
}

func collectRow[ResultType, FlatType any](rows pgx.Rows, flatToResultTypeMapper func(*FlatType) (*ResultType, error)) (*ResultType, error) {
	scannedFlatType, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[FlatType])
	if err != nil {
		return nil, err
	}

	return flatToResultTypeMapper(scannedFlatType)
}
