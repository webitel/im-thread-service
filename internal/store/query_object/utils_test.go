package queryobject

import "testing"

func TestCompactSQL_PreservesSpaceBeforeNamedParam(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// Regression: "@" is a pgx.NamedArgs marker, not an operator. A
			// folded space before it used to produce "select@ThreadID", which
			// after rewriting became "select$1" -> syntax error 42601.
			name: "select before named param",
			in:   "select @ThreadID, @DomainID from s",
			want: "select @ThreadID,@DomainID from s",
		},
		{
			name: "named param after closing paren and keyword",
			in:   "returning * ) select @ThreadID from s",
			want: "returning*)select @ThreadID from s",
		},
		{
			name: "named param after punctuation stays tight",
			in:   "values ((select id from msg_ins), @Phone, @Name)",
			want: "values((select id from msg_ins),@Phone,@Name)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompactSQL(tt.in); got != tt.want {
				t.Fatalf("CompactSQL(%q)\n  got  %q\n  want %q", tt.in, got, tt.want)
			}
		})
	}
}
