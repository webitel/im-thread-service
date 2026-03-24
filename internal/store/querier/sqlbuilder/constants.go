package sqlbuilder

const (
	MessageTable                  string = "im_message.messages"
	MessageButtonInteractionTable string = "im_message.message_button_interactions"
	InteractionPostbackTable      string = "im_message.interaction_postback"
	InteractionContactTable       string = "im_message.interaction_contact"
	InteractionLocationTable string = "im_message.interaction_location"
	MessageLocationAttachment string = "im_message.message_locations"
	MessageContactAttachment string = "im_message.message_contacts"
	MessageDocumentTable string = "im_message.message_documents"
	MessageImageTable string = "im_message.message_images"
	ButtonsCallbackTable string = "im_message.buttons_callback"
	ThreadTable string = "im_thread.thread"
	ThreadDialogTable string = "im_thread.thread_dialog"
)

const (
	IgnoreTag string = "ign"
)
