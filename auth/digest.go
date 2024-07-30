package auth

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type DigestAuth struct {
	realm     string
	nonce     string
	opaque    string
	qop       string
	algorithm string
	hash      hash.Hash
	username  string
	password  string
}

func NewDigestAuth(username string, password string) *DigestAuth {
	return &DigestAuth{
		username: username,
		password: password,
	}
}

func (a *DigestAuth) parse(header http.Header) {
	authHeaders := header.Values("WWW-Authenticate")
	for _, v := range authHeaders {
		if !strings.HasPrefix(v, "Digest ") {
			continue
		}
		v = strings.TrimPrefix(v, "Digest ")
		for _, kv := range strings.Split(v, ",") {
			kv := strings.Split(strings.TrimSpace(kv), "=")
			switch kv[0] {
			case "realm":
				a.realm = strings.Trim(kv[1], `"`)
			case "nonce":
				a.nonce = strings.Trim(kv[1], `"`)
			case "opaque":
				a.opaque = strings.Trim(kv[1], `"`)
			case "qop":
				a.qop = strings.Trim(kv[1], `"`)
			case "algorithm":
				algorithm := strings.Trim(kv[1], `"`)
				switch algorithm {
				case "MD5":
					a.algorithm = "MD5"
					a.hash = md5.New()
				case "SHA-256":
					a.algorithm = "SHA-256"
					a.hash = sha256.New()
				default:
					continue
				}
			}
		}
		break
	}
	if a.algorithm == "" {
		panic("no algorithm")
	}
}

func (a *DigestAuth) HashSum(data string) []byte {
	a.hash.Reset()
	io.WriteString(a.hash, data)
	return a.hash.Sum(nil)
}

func (a *DigestAuth) AuthHeader(req *http.Request) string {
	A1 := fmt.Sprintf("%s:%s:%s", a.username, a.realm, a.password)
	A2 := fmt.Sprintf("%s:%s", req.Method, req.URL.Path)

	HA1 := a.HashSum(A1)
	HA2 := a.HashSum(A2)

	cnonce := a.HashSum(time.Now().String())
	nc := "00000001"

	rawResponse := fmt.Sprintf(
		"%x:%s:%s:%x:%s:%x",
		HA1,
		a.nonce,
		nc,
		cnonce,
		a.qop,
		HA2,
	)

	response := a.HashSum(rawResponse)

	return fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", nc=%s, cnonce="%x", qop=%s, response="%x", algorithm=%s, opaque="%s"`,
		a.username,
		a.realm,
		a.nonce,
		req.URL.Path,
		nc,
		cnonce,
		a.qop,
		response,
		a.algorithm,
		a.opaque,
	)
}

func (a *DigestAuth) Request(method string, url string) (*http.Request, error) {
	log.Printf(`authenticating as user "%s"`, a.username)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		// Not unauthorized, send back the first request
		return req, nil
	}

	a.parse(resp.Header)
	req.Header.Set("Authorization", a.AuthHeader(req))

	return req, nil
}
