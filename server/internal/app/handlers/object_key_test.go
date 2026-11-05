package handlers

import "testing"

func TestObjectKeysMustStayInsideTheirOrgPrefix(t *testing.T) {
	ok := "documents/org-a/sub-1/123-file.pdf"
	if err := validateDocumentObjectKey("org-a", "sub-1", ok); err != nil {
		t.Fatalf("legit key: %v", err)
	}
	for name, key := range map[string]string{
		"other org":        "documents/org-b/sub-1/123-file.pdf",
		"other submission": "documents/org-a/sub-2/123-file.pdf",
		"traversal":        "documents/org-a/sub-1/../../org-b/sub-9/x.pdf",
		"backslash":        "documents/org-a/sub-1/..\\x.pdf",
		"empty name":       "documents/org-a/sub-1/",
		"nul byte":         "documents/org-a/sub-1/a\x00.pdf",
		"empty segment":    "documents/org-a/sub-1//x.pdf",
	} {
		if err := validateDocumentObjectKey("org-a", "sub-1", key); err == nil {
			t.Errorf("document key %s (%q) must be rejected", name, key)
		}
	}
	msgOK := "messages/org-a/hh-1/sw-1/123-a.png"
	if err := validateMessageAttachmentObjectKey("org-a", "hh-1", "sw-1", msgOK); err != nil {
		t.Fatalf("legit message key: %v", err)
	}
	for name, key := range map[string]string{
		"other org":     "messages/org-b/hh-1/sw-1/123-a.png",
		"other swimmer": "messages/org-a/hh-1/sw-2/123-a.png",
		"traversal":     "messages/org-a/hh-1/sw-1/../../../org-b/hh-9/sw-9/x.png",
	} {
		if err := validateMessageAttachmentObjectKey("org-a", "hh-1", "sw-1", key); err == nil {
			t.Errorf("message key %s (%q) must be rejected", name, key)
		}
	}
}
