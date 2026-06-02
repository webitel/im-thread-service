package mapper

import (
	"strconv"

	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func MapToSendDocumentRequest(in *impb.SendDocumentRequest) *dto.SendDocumentRequest {
	if in == nil {
		return nil
	}

	var docReq dto.DocumentRequest

	docReq.Body = in.GetBody()
	docReq.Documents = make([]*dto.Document, 0, len(in.GetDocuments()))

	for _, doc := range in.GetDocuments() {
		id, _ := strconv.ParseInt(doc.GetId(), 10, 64)
		docReq.Documents = append(docReq.Documents, &dto.Document{
			ID:       id,
			Name:     doc.GetFileName(),
			MimeType: doc.GetMimeType(),
			Size:     doc.GetSizeBytes(),
			URL:      doc.GetUrl(),
		})
	}

	sendAs, _ := uuid.Parse(in.GetSendAs())

	return &dto.SendDocumentRequest{
		From:     MapPeerFromProto(in.GetFrom()),
		To:       MapPeerFromProto(in.GetTo()),
		Document: docReq,
		DomainID: in.GetDomainId(),
		SendID:   in.GetSendId(),
		SendAs:   &sendAs,
	}
}

func MapToSendDocumentResponse(out *dto.SendDocumentResponse) *impb.SendDocumentResponse {
	if out == nil {
		return nil
	}

	return &impb.SendDocumentResponse{
		Id: out.ID.String(),
		To: MapPeerToProto(out.To),
	}
}
