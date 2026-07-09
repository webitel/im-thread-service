package queryobject

// Tables & View names
const (
	MessageHistoryView  string = "im_thread.v_messages"
	DirectSettingsTable string = "im_thread.direct_settings"
	ThreadTable         string = "im_thread.thread"
	ThreadDialogTable   string = "im_thread.thread_dialog"
	ContactTable        string = "im_contact.contact"
)

// Default values
const (
	DefaultLimit     int    = 20
	DefaultSortField string = "created_at"
)
