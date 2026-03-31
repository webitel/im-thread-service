package grpc

import "github.com/webitel/im-thread-service/internal/domain/model"


func NewOffsetPagination(size, page int64) *model.OffsetPagination {
	if size <= 0 {
		size =  10
	}
	if page <= 0 {
		page = 1
	}
	return &model.OffsetPagination{
		Size: size,
		Page: page,
	}
}