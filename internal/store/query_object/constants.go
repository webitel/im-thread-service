package queryobject

// Tables & View names
const (
	MessageHistoryView  string = "im_thread.v_messages"
	DirectSettingsTable string = "im_thread.direct_settings"
	ThreadTable         string = "im_thread.thread"
	ThreadDialogTable   string = "im_thread.thread_dialog"
	ThreadVariables     string = "im_thread.thread_variables"
	ThreadTagTable      string = "im_thread.thread_tag"
	ContactTable        string = "im_contact.contact"
	MessageErrorsTable  string = "im_message.message_errors"
)

// Default values
const (
	DefaultLimit     int    = 20
	DefaultSortField string = "created_at"
)
