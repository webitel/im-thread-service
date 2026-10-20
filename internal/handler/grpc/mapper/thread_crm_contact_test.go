package mapper

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

func TestConvertToThread_CRMContact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		crm  string
	}{
		{name: "selected contact is mapped", crm: "42"},
		{name: "no selection stays empty", crm: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := new(ThreadOutConverter).ConvertToThread(&model.Thread{ID: uuid.New(), CRMContactID: tt.crm})
			assert.Equal(t, tt.crm, got.GetCrmContactId())
		})
	}
}
