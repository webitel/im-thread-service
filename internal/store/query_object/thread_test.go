package queryobject

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func TestNewThreadQueryObject(t *testing.T) {
	qo := NewThreadQueryObject()

	assert.NotNil(t, qo)
	assert.NotNil(t, qo.baseQueryObject)
	assert.False(t, qo.mustIncludeComputedSubject)
	assert.Equal(t, 0, qo.join)
}

func TestThreadQueryObject_DefaultFields(t *testing.T) {
	qo := NewThreadQueryObject()
	fields := qo.DefaultFields()

	expectedFields := []string{
		"id", "domain_id", "created_at", "updated_at",
		"kind", "owner", "subject", "description", "members",
	}

	assert.Equal(t, expectedFields, fields)
}

func TestThreadQueryObject_FieldsMetadata(t *testing.T) {
	tests := []struct {
		name                 string
		field                string
		isMembersFilterAdded bool
		wantSortable         bool
		wantRequiresJoin     int
	}{
		{
			name:             "id field",
			field:            "id",
			wantSortable:     true,
			wantRequiresJoin: 0,
		},
		{
			name:             "domain_id field",
			field:            "domain_id",
			wantSortable:     true,
			wantRequiresJoin: 0,
		},
		{
			name:             "subject without members filter",
			field:            "subject",
			wantSortable:     true,
			wantRequiresJoin: 0,
		},
		{
			name:                 "subject with members filter",
			field:                "subject",
			isMembersFilterAdded: true,
			wantSortable:         true,
			wantRequiresJoin:     threadLinkDirectSettings,
		},
		{
			name:             "description field",
			field:            "description",
			wantSortable:     true,
			wantRequiresJoin: 0,
		},
		{
			name:             "members field",
			field:            "members",
			wantSortable:     false,
			wantRequiresJoin: threadLinkFullMembersLateral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.mustIncludeComputedSubject = tt.isMembersFilterAdded

			metadata := qo.FieldsMetadata()
			meta, exists := metadata[tt.field]

			require.True(t, exists, "field %s should exist in metadata", tt.field)
			assert.Equal(t, tt.wantSortable, meta.sortable)
			assert.Equal(t, tt.wantRequiresJoin, meta.requiresJoin)
			assert.NotEmpty(t, meta.sqlExpr)
			assert.NotEmpty(t, meta.aliasedExpr)
			assert.NotEmpty(t, meta.filterExpr)
		})
	}
}

func TestThreadQueryObject_WithIDFilter(t *testing.T) {
	tests := []struct {
		name      string
		ids       []uuid.UUID
		wantWhere bool
	}{
		{
			name:      "empty ids",
			ids:       []uuid.UUID{},
			wantWhere: false,
		},
		{
			name:      "single id",
			ids:       []uuid.UUID{uuid.New()},
			wantWhere: true,
		},
		{
			name:      "multiple ids",
			ids:       []uuid.UUID{uuid.New(), uuid.New()},
			wantWhere: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithIDFilter(tt.ids...)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			if tt.wantWhere {
				assert.Contains(t, sql, "WHERE")
				assert.Contains(t, sql, "t.id")
			}
		})
	}
}

func TestThreadQueryObject_WithDomainIDFilter(t *testing.T) {
	tests := []struct {
		name      string
		domainIDs []int
		wantWhere bool
	}{
		{
			name:      "empty domain ids",
			domainIDs: []int{},
			wantWhere: false,
		},
		{
			name:      "single domain id",
			domainIDs: []int{1},
			wantWhere: true,
		},
		{
			name:      "multiple domain ids",
			domainIDs: []int{1, 2, 3},
			wantWhere: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithDomainIDFilter(tt.domainIDs...)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			if tt.wantWhere {
				assert.Contains(t, sql, "WHERE")
				assert.Contains(t, sql, "t.domain_id")
			}
		})
	}
}

func TestThreadQueryObject_WithKindFilter(t *testing.T) {
	tests := []struct {
		name  string
		kinds []model.ThreadKind
	}{
		{
			name:  "empty kinds",
			kinds: []model.ThreadKind{},
		},
		{
			name:  "single kind",
			kinds: []model.ThreadKind{model.ThreadDirect},
		},
		{
			name:  "multiple kinds",
			kinds: []model.ThreadKind{model.ThreadDirect, model.ThreadGroup},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithKindFilter(tt.kinds...)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			if len(tt.kinds) > 0 {
				assert.Contains(t, sql, "t.kind")
			}
		})
	}
}

func TestThreadQueryObject_WithOwnerFilter(t *testing.T) {
	tests := []struct {
		name   string
		owners []uuid.UUID
	}{
		{
			name:   "empty owners",
			owners: []uuid.UUID{},
		},
		{
			name:   "single owner",
			owners: []uuid.UUID{uuid.New()},
		},
		{
			name:   "multiple owners",
			owners: []uuid.UUID{uuid.New(), uuid.New()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithOwnerFilter(tt.owners...)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			if len(tt.owners) > 0 {
				assert.Contains(t, sql, "t.owner")
			}
		})
	}
}

func TestThreadQueryObject_WithSubjectFilter(t *testing.T) {
	tests := []struct {
		name         string
		subject      string
		wantWhere    bool
		wantContains string
		shouldEscape bool
	}{
		{
			name:      "empty subject",
			subject:   "",
			wantWhere: false,
		},
		{
			name:         "valid subject",
			subject:      "test subject",
			wantWhere:    true,
			wantContains: "t.subject",
		},
		{
			name:         "subject with special LIKE characters",
			subject:      "test%subject_with",
			wantWhere:    true,
			shouldEscape: true,
		},
		{
			name:      "invalid UTF-8",
			subject:   string([]byte{0xff, 0xfe, 0xfd}),
			wantWhere: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithSubjectFilter(tt.subject)

			sql, args, err := qo.ToSql()
			require.NoError(t, err)

			if tt.wantWhere {
				assert.Contains(t, sql, "WHERE")
				assert.Contains(t, sql, "ilike", "should use ILIKE for case-insensitive search")
				if tt.wantContains != "" {
					assert.Contains(t, sql, tt.wantContains)
				}
				if tt.shouldEscape {
					require.Greater(t, len(args), 0)
					_, ok := args[0].(string)
					require.True(t, ok)
				}
			}
		})
	}
}

func TestThreadQueryObject_WithDescriptionFilter(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantWhere   bool
	}{
		{
			name:        "empty description",
			description: "",
			wantWhere:   false,
		},
		{
			name:        "valid description",
			description: "test description",
			wantWhere:   true,
		},
		{
			name:        "invalid UTF-8",
			description: string([]byte{0xff, 0xfe}),
			wantWhere:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithDescriptionFilter(tt.description)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			if tt.wantWhere {
				assert.Contains(t, sql, "t.description")
				assert.Contains(t, sql, "ilike")
			}
		})
	}
}

func TestThreadQueryObject_WithContactIDFilter(t *testing.T) {
	tests := []struct {
		name      string
		memberIDs []uuid.UUID
		wantJoin  bool
	}{
		{
			name:      "empty member ids",
			memberIDs: []uuid.UUID{},
			wantJoin:  false,
		},
		{
			name:      "single member id",
			memberIDs: []uuid.UUID{uuid.New()},
			wantJoin:  true,
		},
		{
			name:      "multiple member ids",
			memberIDs: []uuid.UUID{uuid.New(), uuid.New()},
			wantJoin:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithContactIDFilter(tt.memberIDs...)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			if tt.wantJoin {
				assert.Contains(t, sql, ThreadDialogTable)
				assert.Contains(t, sql, "td.member_id")
				assert.Contains(t, sql, "INNER JOIN "+ThreadDialogTable,
					"hot path must use a regular inner join — no LATERAL loop-in-loop")
				assert.NotContains(t, sql, "LATERAL", "contact filter must stay lightweight (no LATERAL)")
				assert.NotContains(t, sql, "HAVING", "contact filter must stay lightweight (no aggregation)")
				assert.NotContains(t, sql, "WITH ", "contact filter must stay lightweight (no CTE)")
			}
		})
	}
}

func TestThreadQueryObject_WithActiveMembersFilter(t *testing.T) {
	tests := []struct {
		name          string
		memberIDs     []uuid.UUID
		wantFilter    bool
		wantIntersect bool
	}{
		{
			name:       "empty member ids",
			memberIDs:  []uuid.UUID{},
			wantFilter: false,
		},
		{
			name:          "single member id",
			memberIDs:     []uuid.UUID{uuid.New()},
			wantFilter:    true,
			wantIntersect: false,
		},
		{
			name:          "multiple member ids",
			memberIDs:     []uuid.UUID{uuid.New(), uuid.New(), uuid.New()},
			wantFilter:    true,
			wantIntersect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithActiveMembersFilter(tt.memberIDs...)

			sql, args, err := qo.ToSql()
			require.NoError(t, err)

			if !tt.wantFilter {
				assert.NotContains(t, sql, "WITH ")
				assert.NotContains(t, sql, "INTERSECT")
				return
			}

			assert.Contains(t, sql, "WITH "+activeMembersCTE+" AS",
				"thread_ids should be computed up front in a CTE")
			assert.Contains(t, sql, ThreadDialogTable)
			assert.Contains(t, sql, "member_id=")
			assert.Contains(t, sql, "deleted_at IS NULL")
			assert.Contains(t, sql, fmt.Sprintf("SELECT thread_id FROM %s", activeMembersCTE),
				"outer query filters t.id via PK lookup against the CTE result")
			assert.Contains(t, sql, "t.id IN", "outer filter is t.id IN (cte)")
			assert.NotContains(t, sql, "HAVING", "no per-thread aggregation")
			assert.NotContains(t, sql, "LATERAL", "no LATERAL loop-in-loop")
			assert.Equal(t, len(tt.memberIDs), len(args))

			if tt.wantIntersect {
				assert.Contains(t, sql, "INTERSECT")
				assert.Equal(t, len(tt.memberIDs)-1, strings.Count(sql, "INTERSECT"),
					"one INTERSECT between each per-member SELECT")
			} else {
				assert.NotContains(t, sql, "INTERSECT")
			}
		})
	}
}

func TestThreadQueryObject_linkThreadDialog(t *testing.T) {
	t.Run("first call creates join", func(t *testing.T) {
		qo := NewThreadQueryObject()
		qo.linkThreadDialog()

		sql, _, err := qo.ToSql()
		require.NoError(t, err)

		assert.Contains(t, sql, "INNER JOIN "+ThreadDialogTable)
		assert.Contains(t, sql, "td.thread_id=t.id")
		assert.NotContains(t, sql, "LATERAL", "must be a regular inner join, not a LATERAL loop-in-loop")
		assert.Equal(t, threadLinkThreadDialog, qo.join&threadLinkThreadDialog)
	})

	t.Run("second call does not duplicate join", func(t *testing.T) {
		qo := NewThreadQueryObject()
		qo.linkThreadDialog()
		qo.linkThreadDialog()

		sql, _, err := qo.ToSql()
		require.NoError(t, err)

		assert.Equal(t, threadLinkThreadDialog, qo.join&threadLinkThreadDialog, "bitflag should be set exactly once")
		assert.Equal(t, 1, strings.Count(sql, "INNER JOIN "+ThreadDialogTable),
			"thread_dialog inner join should appear exactly once")
	})
}

func TestThreadQueryObject_linkDirectSettings(t *testing.T) {
	t.Run("creates both thread dialog and direct settings joins", func(t *testing.T) {
		qo := NewThreadQueryObject()
		qo.linkDirectSettings()

		sql, _, err := qo.ToSql()
		require.NoError(t, err)

		assert.Contains(t, sql, "INNER JOIN")
		assert.Contains(t, sql, ThreadDialogTable)
		assert.Contains(t, sql, "LEFT JOIN")
		assert.Contains(t, sql, DirectSettingsTable)
		assert.Contains(t, sql, "ds.thread_dialog_id=td.id")
		assert.Equal(t, threadLinkDirectSettings, qo.join&threadLinkDirectSettings)
		assert.Equal(t, threadLinkThreadDialog, qo.join&threadLinkThreadDialog)
	})

	t.Run("does not duplicate joins", func(t *testing.T) {
		qo := NewThreadQueryObject()
		qo.linkDirectSettings()
		qo.linkDirectSettings()

		sql, _, err := qo.ToSql()
		require.NoError(t, err)

		leftJoinCount := strings.Count(sql, "LEFT JOIN "+DirectSettingsTable)
		assert.Equal(t, 1, leftJoinCount)
	})
}

func TestThreadQueryObject_linkFullMembersLateral(t *testing.T) {
	t.Run("creates lateral join for full members", func(t *testing.T) {
		qo := NewThreadQueryObject()
		qo.linkFullMembersLateral()

		sql, _, err := qo.ToSql()
		require.NoError(t, err)

		assert.Contains(t, sql, "LEFT JOIN")
		assert.Contains(t, sql, "lateral")
		assert.Contains(t, sql, "jsonb_agg")
		assert.Contains(t, sql, "jsonb_build_object")
		assert.Contains(t, sql, "members_data")
		assert.Equal(t, threadLinkFullMembersLateral, qo.join&threadLinkFullMembersLateral)
	})
}

func TestThreadQueryObject_EnsureJoins(t *testing.T) {
	tests := []struct {
		name         string
		requiredJoin int
		wantJoins    []string
	}{
		{
			name:         "thread dialog join",
			requiredJoin: threadLinkThreadDialog,
			wantJoins:    []string{"INNER JOIN", ThreadDialogTable},
		},
		{
			name:         "direct settings join",
			requiredJoin: threadLinkDirectSettings,
			wantJoins:    []string{"INNER JOIN", "LEFT JOIN", DirectSettingsTable},
		},
		{
			name:         "full members lateral join",
			requiredJoin: threadLinkFullMembersLateral,
			wantJoins:    []string{"LEFT JOIN", "lateral", "jsonb_agg"},
		},
		{
			name:         "multiple joins",
			requiredJoin: threadLinkThreadDialog | threadLinkDirectSettings,
			wantJoins:    []string{"INNER JOIN", "LEFT JOIN", DirectSettingsTable},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.EnsureJoins(tt.requiredJoin)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			for _, want := range tt.wantJoins {
				assert.Contains(t, sql, want)
			}
		})
	}
}

func TestThreadQueryObject_WithFields(t *testing.T) {
	tests := []struct {
		name         string
		fields       []string
		wantInSQL    []string
		wantNotInSQL []string
	}{
		{
			name:      "basic fields",
			fields:    []string{"id", "domain_id", "created_at"},
			wantInSQL: []string{"t.id", "t.domain_id", "t.created_at"},
		},
		{
			name:      "field requiring join",
			fields:    []string{"members"},
			wantInSQL: []string{"members_data", "lateral", "jsonb_agg"},
		},
		{
			name:         "invalid field ignored",
			fields:       []string{"id", "invalid_field", "domain_id"},
			wantInSQL:    []string{"t.id", "t.domain_id"},
			wantNotInSQL: []string{"invalid_field"},
		},
		{
			name:      "empty fields",
			fields:    []string{},
			wantInSQL: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithFields(tt.fields)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			for _, want := range tt.wantInSQL {
				assert.Contains(t, sql, want)
			}

			for _, notWant := range tt.wantNotInSQL {
				assert.NotContains(t, sql, notWant)
			}
		})
	}
}

func TestThreadQueryObject_WithSort(t *testing.T) {
	tests := []struct {
		name       string
		sortFields []string
		wantInSQL  []string
	}{
		{
			name:       "ascending sort",
			sortFields: []string{"+created_at"},
			wantInSQL:  []string{"ORDER BY", "t.created_at ASC"},
		},
		{
			name:       "descending sort",
			sortFields: []string{"-updated_at"},
			wantInSQL:  []string{"ORDER BY", "t.updated_at DESC"},
		},
		{
			name:       "multiple sort fields",
			sortFields: []string{"+created_at", "-updated_at"},
			wantInSQL:  []string{"ORDER BY", "t.created_at ASC", "t.updated_at DESC"},
		},
		{
			name:       "invalid sort field ignored",
			sortFields: []string{"+created_at", "+invalid_field"},
			wantInSQL:  []string{"t.created_at ASC"},
		},
		{
			name:       "non-sortable field ignored",
			sortFields: []string{"+member_ids"},
			wantInSQL:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithFields([]string{"id"}) // Need at least one field
			qo.WithSort(tt.sortFields...)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			for _, want := range tt.wantInSQL {
				assert.Contains(t, sql, want)
			}
		})
	}
}

func TestThreadQueryObject_WithLimit(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		quota     []int
		wantLimit uint64
	}{
		{
			name:      "positive limit",
			limit:     10,
			wantLimit: 11, // +1 для перевірки hasNext
		},
		{
			name:      "negative limit uses default",
			limit:     -1,
			wantLimit: uint64(DefaultLimit) + 1,
		},
		{
			name:      "limit with quota - limit smaller",
			limit:     10,
			quota:     []int{100},
			wantLimit: 11,
		},
		{
			name:      "limit with quota - quota smaller",
			limit:     100,
			quota:     []int{10},
			wantLimit: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithFields([]string{"id"})
			qo.WithLimit(tt.limit, tt.quota...)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			assert.Contains(t, sql, "LIMIT")
		})
	}
}

func TestThreadQueryObject_WithOffset(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		limit      int
		wantOffset bool
	}{
		{
			name:       "page 1 - no offset",
			page:       1,
			limit:      10,
			wantOffset: false,
		},
		{
			name:       "page 2 - with offset",
			page:       2,
			limit:      10,
			wantOffset: true,
		},
		{
			name:       "page 0 - no offset",
			page:       0,
			limit:      10,
			wantOffset: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qo := NewThreadQueryObject()
			qo.WithFields([]string{"id"})
			qo.WithLimit(tt.limit)
			qo.WithOffset(tt.page)

			sql, _, err := qo.ToSql()
			require.NoError(t, err)

			if tt.wantOffset {
				assert.Contains(t, sql, "OFFSET")
			} else {
				assert.NotContains(t, sql, "OFFSET")
			}
		})
	}
}

func TestThreadQueryObject_ComplexQuery(t *testing.T) {
	t.Run("full query with all filters and joins", func(t *testing.T) {
		qo := NewThreadQueryObject()

		threadID := uuid.New()
		memberID := uuid.New()
		ownerID := uuid.New()

		qo.WithFields([]string{"id", "subject", "member_ids", "members"}).
			WithIDFilter(threadID).
			WithDomainIDFilter(1).
			WithKindFilter(model.ThreadDirect).
			WithOwnerFilter(ownerID).
			WithSubjectFilter("test").
			WithContactIDFilter(memberID).
			WithSort("+created_at", "-updated_at").
			WithLimit(20).
			WithOffset(2)

		sql, args, err := qo.ToSql()
		require.NoError(t, err)

		// Verify structure
		assert.Contains(t, sql, "SELECT")
		assert.Contains(t, sql, "FROM")
		assert.Contains(t, sql, "WHERE")
		assert.Contains(t, sql, "ORDER BY")
		assert.Contains(t, sql, "LIMIT")
		assert.Contains(t, sql, "OFFSET")

		// Verify joins
		assert.Contains(t, sql, "INNER JOIN")
		assert.Contains(t, sql, "LEFT JOIN")
		assert.Contains(t, sql, "lateral")

		// Verify args
		assert.Greater(t, len(args), 0)

		t.Logf("Generated SQL:\n%s", sql)
		t.Logf("Args: %v", args)
	})
}

func TestThreadQueryObject_SubjectWithMembersFilter(t *testing.T) {
	t.Run("subject expression unchanged with members filter", func(t *testing.T) {
		qo := NewThreadQueryObject()
		memberID := uuid.New()

		qo.WithFields([]string{"subject"}).
			WithContactIDFilter(memberID)

		sql, _, err := qo.ToSql()
		require.NoError(t, err)

		// WithContactIDFilter alone should NOT trigger the computed subject expression
		assert.Contains(t, sql, "t.subject")
		assert.NotContains(t, sql, "ds.title")
		assert.NotContains(t, sql, DirectSettingsTable)
		// But the thread_dialog join and member filter should be present
		assert.Contains(t, sql, "INNER JOIN "+ThreadDialogTable)
		assert.Contains(t, sql, "td.member_id")
	})

	t.Run("subject expression without members filter", func(t *testing.T) {
		qo := NewThreadQueryObject()

		qo.WithFields([]string{"subject"})

		sql, _, err := qo.ToSql()
		require.NoError(t, err)

		// Should use simple subject expression
		assert.Contains(t, sql, "t.subject")
		assert.NotContains(t, sql, "case")
		assert.NotContains(t, sql, DirectSettingsTable)
	})
}

func TestThreadQueryObject_ChainedFilters(t *testing.T) {
	t.Run("method chaining works correctly", func(t *testing.T) {
		qo := NewThreadQueryObject()

		result := qo.
			WithIDFilter(uuid.New()).
			WithDomainIDFilter(1).
			WithKindFilter(model.ThreadDirect)

		assert.NotNil(t, result)
		assert.Equal(t, qo, result, "should return same instance for chaining")
	})
}
