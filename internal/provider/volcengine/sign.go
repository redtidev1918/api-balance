// Package volcengine implements the Volcengine Ark Coding Plan / Agent Plan
// quota providers via the OpenAPI (GetCodingPlanUsage / GetAgentPlanAFPUsage).
//
// Auth is Volcano Cloud AccessKeyId + SecretAccessKey signed with
// HMAC-SHA256 V4 (region cn-beijing, service ark). These endpoints are
// undocumented/private and are therefore marked experimental: they may change
// or vanish as Volcengine updates its plan billing.
package volcengine

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	region  = "cn-beijing"
	service = "ark"
	host    = "open.volcengineapi.com"
)

// signedHeaders canonical set used by the Volcengine V4 signature.
const signedHeaders = "content-type;host;x-content-sha256;x-date"

// signRequest builds the Authorization header for a Volcengine OpenAPI call
// using AK/SK + HMAC-SHA256 V4. payload is the (UTF-8) request body, typically
// "{}". It follows the exact scheme used by the official SDK and community
// implementations (see dsh-volcark-quota).
func signRequest(ak, sk string, xDate time.Time, action, version string, payload []byte) (auth string, bodyHash string) {
	bodyHash = sha256Hex(payload)
	xDateStr := xDate.UTC().Format("20060102T150405Z")
	xDateShort := xDate.UTC().Format("20060102")

	query := url.Values{}
	query.Set("Action", action)
	query.Set("Version", version)
	qstr := normalizedQuery(query)

	canonicalHeaders := "content-type:application/json\n" +
		"host:" + host + "\n" +
		"x-content-sha256:" + bodyHash + "\n" +
		"x-date:" + xDateStr + "\n"
	canonicalRequest := strings.Join([]string{
		"POST", "/", qstr, canonicalHeaders, signedHeaders, bodyHash,
	}, "\n")

	scope := xDateShort + "/" + region + "/" + service + "/request"
	hashedRequest := sha256Hex([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{"HMAC-SHA256", xDateStr, scope, hashedRequest}, "\n")

	kDate := hmacSHA(sk, xDateShort)
	kRegion := hmacSHA(string(kDate), region)
	kService := hmacSHA(string(kRegion), service)
	kSigning := hmacSHA(string(kService), "request")
	signature := hex.EncodeToString(hmacSHA(string(kSigning), stringToSign))

	auth = "HMAC-SHA256 Credential=" + ak + "/" + scope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
	return auth, bodyHash
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA(key, data string) []byte {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(data))
	return h.Sum(nil)
}

func normalizedQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(q.Get(k)))
	}
	return strings.Join(parts, "&")
}