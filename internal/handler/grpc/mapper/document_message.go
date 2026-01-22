package mapper

import (
	"strconv"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func MapToSendDocumentRequest(in *impb.SendDocumentRequest) *dto.SendDocumentRequest {
	if in == nil {
		return nil
	}

	var docReq dto.DocumentRequest
	if pbDoc := in.GetDocument(); pbDoc != nil {
		docReq.Body = pbDoc.GetBody()
		docReq.Documents = make([]*dto.Document, 0, len(pbDoc.GetDocuments()))

		for _, doc := range pbDoc.GetDocuments() {
			id, _ := strconv.ParseInt(doc.GetId(), 10, 64)
			docReq.Documents = append(docReq.Documents, &dto.Document{
				ID:       id,
				Name:     doc.GetFileName(),
				MimeType: doc.GetMimeType(),
				Size:     doc.GetSizeBytes(),
			})
		}
	}

	return &dto.SendDocumentRequest{
		From:     MapPeerFromProto(in.GetFrom()),
		To:       MapPeerFromProto(in.GetTo()),
		Document: docReq,
		DomainID: in.GetDomainId(),
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
