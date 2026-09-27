package sealed

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func blob(n int) []byte {
	b := bytes.Repeat([]byte{0xab}, n)
	b[0] = BlobVersion
	return b
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func pub() []byte { return bytes.Repeat([]byte{7}, PublicKeyLen) }

func wantInvalid(t *testing.T, err error, field string) {
	t.Helper()
	var inv *InvalidError
	if !errors.As(err, &inv) || inv.Field != field {
		t.Fatalf("want invalid %q, got %v", field, err)
	}
}

func TestDecodeBase64url(t *testing.T) {
	raw := []byte{0xfb, 0xff, 0xfe, 0x01}
	for _, s := range []string{"-__-AQ", "-__-AQ=="} {
		got, err := Decode(s)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("Decode(%q) = %x, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "+//+AQ", "-__- AQ", "-__-A"} {
		if _, err := Decode(s); err == nil {
			t.Fatalf("Decode(%q) must fail", s)
		}
	}
	if Encode(raw) != "-__-AQ" {
		t.Fatalf("Encode = %s", Encode(raw))
	}
}

func TestDecodePublicKey(t *testing.T) {
	if got, err := DecodePublicKey("pk", b64(pub())); err != nil || len(got) != 32 {
		t.Fatalf("valid key: %v", err)
	}
	wantInvalid(t, func() error { _, err := DecodePublicKey("pk", b64(pub()[:31])); return err }(), "pk")
	wantInvalid(t, func() error { _, err := DecodePublicKey("pk", b64(append(pub(), 1))); return err }(), "pk")
	wantInvalid(t, func() error { _, err := DecodePublicKey("pk", b64(make([]byte, 32))); return err }(), "pk")
	wantInvalid(t, func() error { _, err := DecodePublicKey("pk", "not base64!"); return err }(), "pk")
}

// le32 encodes u as a 32-byte little-endian X25519 u-coordinate.
func le32(t *testing.T, u *big.Int) []byte {
	t.Helper()
	be := u.FillBytes(make([]byte, 32))
	out := make([]byte, 32)
	for i, c := range be {
		out[31-i] = c
	}
	return out
}

func TestDecodePublicKeySmallOrderAndNonCanonical(t *testing.T) {
	dec := func(s string) *big.Int {
		v, _ := new(big.Int).SetString(s, 10)
		return v
	}
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	add := func(a *big.Int, n int64) *big.Int { return new(big.Int).Add(a, big.NewInt(n)) }
	topBit := pub()
	topBit[31] |= 0x80
	cases := []struct {
		name    string
		key     []byte
		problem string // "" = accepted
	}{
		{"zero", le32(t, big.NewInt(0)), "low-order key"},
		{"one", le32(t, big.NewInt(1)), "low-order key"},
		{"p-1", le32(t, add(p, -1)), "low-order key"},
		{"order 8 (a)", le32(t, dec("325606250916557431795983626356110631294008115727848805560023387167927233504")), "low-order key"},
		{"order 8 (b)", le32(t, dec("39382357235489614581723060781553021112529911719440698176882885853963445705823")), "low-order key"},
		{"p (alias of 0)", le32(t, p), "non-canonical key (u >= p)"},
		{"p+1 (alias of 1)", le32(t, add(p, 1)), "non-canonical key (u >= p)"},
		{"2^255-1", le32(t, add(p, 18)), "non-canonical key (u >= p)"},
		{"top bit set", topBit, "non-canonical key (top bit set)"},
		{"top bit on zero", func() []byte { b := make([]byte, 32); b[31] = 0x80; return b }(), "non-canonical key (top bit set)"},
		{"base point 9", le32(t, big.NewInt(9)), ""},
		{"p-2", le32(t, add(p, -2)), ""},
		{"ordinary", pub(), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DecodePublicKey("pk", b64(c.key))
			if c.problem == "" {
				if err != nil || !bytes.Equal(got, c.key) {
					t.Fatalf("want accepted, got %v", err)
				}
				return
			}
			var inv *InvalidError
			if !errors.As(err, &inv) || inv.Field != "pk" || inv.Problem != c.problem {
				t.Fatalf("want invalid pk %q, got %v", c.problem, err)
			}
		})
	}
}

func TestBlobBounds(t *testing.T) {
	cases := []struct {
		name   string
		decode func(string, string) ([]byte, error)
		lo, hi int
	}{
		{"wrap", DecodeWrap, MinWrap, MaxWrap},
		{"private", DecodePrivateSealed, MinPrivateSealed, MaxPrivateSealed},
		{"entry", DecodeEntrySealed, MinEntrySealed, MaxEntrySealed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, n := range []int{c.lo, c.hi} {
				if _, err := c.decode("f", b64(blob(n))); err != nil {
					t.Fatalf("%d bytes must pass: %v", n, err)
				}
			}
			for _, n := range []int{c.lo - 1, c.hi + 1} {
				wantInvalid(t, func() error { _, err := c.decode("f", b64(blob(n))); return err }(), "f")
			}
			bad := blob(c.lo)
			bad[0] = 0x02
			_, err := c.decode("f", b64(bad))
			wantInvalid(t, err, "f")
			if !strings.Contains(err.Error(), "version") {
				t.Fatalf("unknown version must be named: %v", err)
			}
		})
	}
	if MinWrap != 93 || MinPrivateSealed != 61 || MinEntrySealed != 30 {
		t.Fatal("minimum sizes must match the P2.6 wire formats")
	}
}

func TestKDFBounds(t *testing.T) {
	salt := b64(bytes.Repeat([]byte{1}, KDFSaltLen))
	ok := KDF{Alg: "argon2id", M: 64 * 1024, T: 3, P: 1, Salt: salt + "=="}
	got, err := ok.Validate("kdf")
	if err != nil || got.Salt != salt {
		t.Fatalf("contract params must pass and canonicalise the salt: %+v %v", got, err)
	}
	for _, edge := range []KDF{
		{Alg: "argon2id", M: MinKDFMemoryKiB, T: MinKDFTime, P: MinKDFThreads, Salt: salt},
		{Alg: "argon2id", M: MaxKDFMemoryKiB, T: MaxKDFTime, P: MaxKDFThreads, Salt: salt},
	} {
		if _, err := edge.Validate("kdf"); err != nil {
			t.Fatalf("edge %+v: %v", edge, err)
		}
	}
	bad := map[string]KDF{
		"kdf.alg":  {Alg: "scrypt", M: 65536, T: 3, P: 1, Salt: salt},
		"kdf.m":    {Alg: "argon2id", M: MinKDFMemoryKiB - 1, T: 3, P: 1, Salt: salt},
		"kdf.t":    {Alg: "argon2id", M: 65536, T: 11, P: 1, Salt: salt},
		"kdf.p":    {Alg: "argon2id", M: 65536, T: 3, P: 0, Salt: salt},
		"kdf.salt": {Alg: "argon2id", M: 65536, T: 3, P: 1, Salt: b64(make([]byte, 15))},
	}
	for field, k := range bad {
		_, err := k.Validate("kdf")
		wantInvalid(t, err, field)
	}
	if _, err := (KDF{Alg: "argon2id", M: MaxKDFMemoryKiB + 1, T: 3, P: 1, Salt: salt}).Validate("kdf"); err == nil {
		t.Fatal("memory above 256 MiB must fail")
	}
}

func TestRecoveryFingerprint(t *testing.T) {
	p := pub()
	fp := Fingerprint(p)
	if len(fp) != 16 || strings.ToLower(fp) != fp {
		t.Fatalf("fingerprint %q", fp)
	}
	in := RecoveryInput{PublicKey: b64(p), Fingerprint: strings.ToUpper(fp), Wrap: b64(blob(MinWrap))}
	if _, err := in.parse("recovery"); err != nil {
		t.Fatalf("valid recovery (case-insensitive fingerprint): %v", err)
	}
	in.Fingerprint = "0000000000000000"
	_, err := in.parse("recovery")
	wantInvalid(t, err, "recovery.fingerprint")
}

func TestParseWraps(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	w := b64(blob(MinWrap))
	good := []WrapInput{{RecipientKind: KindMember, RecipientID: &a, Wrap: w}, {RecipientKind: KindDevice, RecipientID: &a, Wrap: w}}
	if got, err := parseWraps("wraps", good); err != nil || len(got) != 2 {
		t.Fatalf("member and device with the same id are distinct recipients: %v", err)
	}
	cases := map[string][]WrapInput{
		"wraps[0].recipient_kind": {{RecipientKind: KindRecovery, Wrap: w}},
		"wraps[0].recipient_id":   {{RecipientKind: KindMember, Wrap: w}},
		"wraps[1].recipient_id":   {{RecipientKind: KindMember, RecipientID: &b, Wrap: w}, {RecipientKind: KindMember, RecipientID: &b, Wrap: w}},
		"wraps[0].wrap":           {{RecipientKind: KindMember, RecipientID: &b, Wrap: b64(blob(MinWrap - 1))}},
	}
	for field, in := range cases {
		_, err := parseWraps("wraps", in)
		wantInvalid(t, err, field)
	}
	many := make([]WrapInput, MaxWraps+1)
	_, err := parseWraps("wraps", many)
	wantInvalid(t, err, "wraps")
}

func TestWrapInputAliasesAndUnknownFields(t *testing.T) {
	id := uuid.New()
	var w WrapInput
	if err := json.Unmarshal([]byte(`{"kind":"device","recipient_id":"`+id.String()+`","wrap_sealed":"AQ"}`), &w); err != nil {
		t.Fatal(err)
	}
	if w.RecipientKind != KindDevice || *w.RecipientID != id || w.Wrap != "AQ" {
		t.Fatalf("aliases not applied: %+v", w)
	}
	if err := json.Unmarshal([]byte(`{"recipient_kind":"member","recipient_id":"`+id.String()+`","wrap":"AQ","extra":1}`), &w); err == nil {
		t.Fatal("unknown fields must be refused")
	}
}

func TestBanInputExpiryWindow(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	entry := b64(blob(64))
	in := func(exp time.Time) BanInput { return BanInput{KeyVersion: 1, EntrySealed: entry, ExpiresAt: exp} }
	for _, exp := range []time.Time{
		now.Add(24 * time.Hour),
		now.Add(24*time.Hour - time.Minute), // client clock / latency grace
		now.AddDate(3, 0, 0),
	} {
		if _, err := in(exp).parse(now); err != nil {
			t.Fatalf("expiry %s must pass: %v", exp, err)
		}
	}
	for _, exp := range []time.Time{{}, now, now.Add(23 * time.Hour), now.AddDate(3, 0, 1)} {
		_, err := in(exp).parse(now)
		wantInvalid(t, err, "expires_at")
	}
	_, err := BanInput{EntrySealed: entry, ExpiresAt: now.AddDate(0, 1, 0)}.parse(now)
	wantInvalid(t, err, "key_version")
	_, err = BanInput{KeyVersion: 1, EntrySealed: b64(blob(MaxEntrySealed + 1)), ExpiresAt: now.AddDate(0, 1, 0)}.parse(now)
	wantInvalid(t, err, "entry_sealed")
}

func TestMemberKeyInput(t *testing.T) {
	in := MemberKeyInput{
		PublicKey: b64(pub()), PrivateSealed: b64(blob(MinPrivateSealed)),
		KDF: KDF{Alg: "argon2id", M: 65536, T: 3, P: 1, Salt: b64(make([]byte, 16))},
	}
	if _, err := in.parse(); err != nil {
		t.Fatal(err)
	}
	in.PrivateSealed = b64(blob(MinPrivateSealed - 1))
	_, err := in.parse()
	wantInvalid(t, err, "private_sealed")
}
