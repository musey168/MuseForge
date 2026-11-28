package domain

import (
	"strings"
	"testing"
)

func TestIssueCloseSource(t *testing.T) {
	manual := Issue{Status: "closed", CloseSource: "manual"}
	if !manual.ManuallyClosed() || manual.TrackerClosed() {
		t.Fatalf("manual close classification: %+v", manual)
	}
	legacy := Issue{Status: "closed"}
	if !legacy.ManuallyClosed() {
		t.Fatal("older manually closed issues must remain manual")
	}
	external := Issue{Source: "github", Status: "closed", CloseSource: "external", ExternalRef: &ExternalRef{ExternalState: "已关闭"}}
	if external.ManuallyClosed() || !external.TrackerClosed() {
		t.Fatalf("external close classification: %+v", external)
	}
	if !strings.Contains(external.ExecutionBlockedReason(), "外部系统") {
		t.Fatalf("external close block reason = %q", external.ExecutionBlockedReason())
	}
}

func TestMergedDeliveryBlocksUntilReopened(t *testing.T) {
	merged := Issue{Source: "github", DeliveryState: "merged"}
	if !merged.DeliveryLocked() || merged.ExecutionBlockedReason() == "" {
		t.Fatal("merged delivery must block execution")
	}
	if !strings.Contains(merged.ExecutionBlockedReason(), "交付闭环") {
		t.Fatalf("expected delivery-closed wording, got %q", merged.ExecutionBlockedReason())
	}
	if merged.DeliveryLockedLabel() != "交付闭环" {
		t.Fatalf("expected delivery-closed label, got %q", merged.DeliveryLockedLabel())
	}
	closed := Issue{Status: "closed", DeliveryState: "merged"}
	if closed.DeliveryLockedLabel() != "关闭" || !strings.Contains(closed.ExecutionBlockedReason(), "工作项已关闭") {
		t.Fatalf("persisted manual close must win: label=%q reason=%q", closed.DeliveryLockedLabel(), closed.ExecutionBlockedReason())
	}
	merged.Status = "done"
	merged.DeliveryState = "reopened"
	if merged.DeliveryLocked() || merged.ExecutionBlockedReason() != "" {
		t.Fatal("explicitly reopened delivery must be triggerable")
	}
}
