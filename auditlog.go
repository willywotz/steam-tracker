package steamtracker

import (
	"fmt"
	"time"
)

type AuditLog struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Raw       JSON      `json:"raw" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
}

func NewAuditLogFromString(raw string) *AuditLog {
	return &AuditLog{
		Raw: JSON(raw),
	}
}

func (al *AuditLog) MarshalJSON() ([]byte, error) {
	if al == nil {
		return []byte("null"), nil
	}
	buf := make([]byte, 0)
	buf = append(buf, '{')
	buf = append(buf, `"audit_id":`...)
	buf = append(buf, fmt.Sprintf("%d", al.ID)...)
	buf = append(buf, `,"audit_created_at":"`...)
	buf = append(buf, al.CreatedAt.Format(time.RFC3339)...)
	buf = append(buf, '"')
	if al.Raw != nil {
		buf = append(buf, ',')
		buf = append(buf, al.Raw[:len(al.Raw)-1][1:]...)
	}
	buf = append(buf, '}')
	return buf, nil
}

type CreateAuditLogCommand struct {
	Raw JSON `json:"raw"`
}

func (cmd *CreateAuditLogCommand) AuditLog() AuditLog {
	return AuditLog{
		Raw: cmd.Raw,
	}
}

type SearchAuditLogsQuery struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	SortBy struct {
		ID *string `json:"id"`
	} `json:"sort_by"`
}

func (query *SearchAuditLogsQuery) Validate() error {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit < 1 || query.Limit > 100 {
		query.Limit = 25
	}

	if query.SortBy.ID != nil {
		if *query.SortBy.ID != "asc" && *query.SortBy.ID != "desc" {
			return fmt.Errorf("invalid sort order for id: %s, must be 'asc' or 'desc'", *query.SortBy.ID)
		}
	}

	return nil
}

type SearchAuditLogsQueryResult struct {
	TotalCount int64 `json:"total_count"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`

	AuditLogs []*AuditLog `json:"audit_logs"`
}
