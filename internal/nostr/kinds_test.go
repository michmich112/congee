package nostr

import "testing"

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		kind int
		want KindClass
	}{
		{0, KindReplaceable},
		{3, KindReplaceable},
		{10001, KindReplaceable},
		{1, KindRegular},
		{2, KindRegular},
		{4, KindRegular},
		{44, KindRegular},
		{1000, KindRegular},
		{9999, KindRegular},
		{20000, KindEphemeral},
		{25000, KindEphemeral},
		{29999, KindEphemeral},
		{30000, KindAddressable},
		{35000, KindAddressable},
		{500, KindRegular},
	}
	for _, tc := range cases {
		if g := ClassifyKind(tc.kind); g != tc.want {
			t.Fatalf("kind %d: got %v want %v", tc.kind, g, tc.want)
		}
	}
}
