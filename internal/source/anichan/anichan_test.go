package anichan

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestDecryptPayloadRoundtrip(t *testing.T) {
	keys := keyTriple{
		k: "8fa71891",
		o: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		b: base64.StdEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210")),
	}
	token := "session-token-123"
	rawO, _ := base64.StdEncoding.DecodeString(keys.o)
	rawB, _ := base64.StdEncoding.DecodeString(keys.b)
	hmacKey := make([]byte, len(rawO))
	for i := range rawO {
		hmacKey[i] = rawO[i] ^ rawB[i]
	}
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write([]byte(token))
	aesKey := mac.Sum(nil)

	payload, _ := json.Marshal(map[string]any{"servers": []map[string]any{{"name": "anikoto", "type": "hls"}}})
	block, _ := aes.NewCipher(aesKey)
	gcm, _ := cipher.NewGCM(block)
	iv := []byte("123456789012")
	ct := gcm.Seal(nil, iv, payload, nil)
	wrapped, _ := json.Marshal(map[string]string{
		"v": "1",
		"i": base64.StdEncoding.EncodeToString(iv),
		"d": base64.StdEncoding.EncodeToString(ct),
	})
	// v must be a number in the real payload
	var wrappedMap map[string]any
	_ = json.Unmarshal(wrapped, &wrappedMap)
	wrappedMap["v"] = 1
	wrapped, _ = json.Marshal(wrappedMap)

	plain, err := decryptPayload(wrapped, token, keys)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), "anikoto") {
		t.Errorf("hasil dekripsi salah: %s", plain)
	}

	// Unencrypted payloads pass through untouched.
	plainRaw, err := decryptPayload([]byte(`{"servers":[]}`), token, keys)
	if err != nil || string(plainRaw) != `{"servers":[]}` {
		t.Errorf("payload polos harus lolos apa adanya: %s, %v", plainRaw, err)
	}
}

func TestUnwrapStream(t *testing.T) {
	c := New("")
	// AniChan-native relative stream
	d, r := c.unwrapStream("/api/watch/m3u8?sh=1%2F1%2Fsub%2Fmaster.m3u8&sig=x")
	if d != "https://anichan.to/api/watch/m3u8?sh=1%2F1%2Fsub%2Fmaster.m3u8&sig=x" || r != "https://anichan.to/" {
		t.Errorf("relatif: %q %q", d, r)
	}
	// Wrapper with embedded upstream + ref
	d, r = c.unwrapStream("https://c.toroplay.cymru/api/watch/m3u8?url=https%3A%2F%2Fcdn.example%2Fmaster.m3u8&ref=https%3A%2F%2Fref.example%2F")
	if d != "https://cdn.example/master.m3u8" || r != "https://ref.example/" {
		t.Errorf("wrapper: %q %q", d, r)
	}
	// Wrapper without ref defaults to megaplay referer
	d, r = c.unwrapStream("https://c.toroplay.cymru/api/watch/m3u8?url=https%3A%2F%2Fcdn.example%2Fm.m3u8")
	if r != "https://megaplay.buzz/" {
		t.Errorf("wrapper tanpa ref: %q", r)
	}
	// Plain direct URL
	d, r = c.unwrapStream("https://cdn.example/plain.m3u8")
	if d != "https://cdn.example/plain.m3u8" || r != "https://anichan.to/" {
		t.Errorf("polos: %q %q", d, r)
	}
}

func TestServerPriority(t *testing.T) {
	if !(serverPriority("anikoto") < serverPriority("megaplay_hd1") &&
		serverPriority("megaplay_hd1") < serverPriority("megaplay_hd2") &&
		serverPriority("megaplay_hd2") < serverPriority("aniwatchtv") &&
		serverPriority("aniwatchtv") < serverPriority("anichan")) {
		t.Error("urutan prioritas server salah")
	}
}

func TestTripleRegex(t *testing.T) {
	o := "CEmlWYs/MedMwv4fL8bc" + "12345678901234567890123" + "=" // 44 chars
	b := "1SRhkCEhn5/o6lh1rF34" + "12345678901234567890123" + "=" // 44 chars
	if len(o) != 44 || len(b) != 44 {
		t.Fatalf("fixture harus 44 karakter: %d %d", len(o), len(b))
	}
	js := []byte(`xx="x",k1="8fa71891",o1="` + o + `",b1="` + b + `";`)
	m := tripleRe.FindSubmatch(js)
	if m == nil || string(m[1]) != "8fa71891" {
		t.Errorf("regex triple gagal: %v", m)
	}
}
