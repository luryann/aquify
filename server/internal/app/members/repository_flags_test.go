package members

import "testing"

func TestDecodeMedicalFlagsReturnsCorruptDataError(t *testing.T) {
	if _, err := decodeMedicalFlags(`{"asthma":true}`); err == nil {
		t.Fatal("expected a decode error for non-array medical_flags")
	}
	flags, err := decodeMedicalFlags(`["asthma","epipen"]`)
	if err != nil || len(flags) != 2 {
		t.Fatalf("flags=%v err=%v", flags, err)
	}
	if flags, err := decodeMedicalFlags(`null`); err != nil || len(flags) != 0 {
		t.Fatalf("null flags=%v err=%v", flags, err)
	}
}

func TestEncodeMedicalFlagsNilIsEmptyArray(t *testing.T) {
	out, err := encodeMedicalFlags(nil)
	if err != nil || string(out) != "[]" {
		t.Fatalf("out=%s err=%v", out, err)
	}
}
