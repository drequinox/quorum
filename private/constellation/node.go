package constellation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tv42/httpunix"
)

func launchNode(cfgPath string) (*exec.Cmd, error) {
	cmd := exec.Command("constellation-node", cfgPath)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go io.Copy(os.Stderr, stderr)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	time.Sleep(100 * time.Millisecond)
	return cmd, nil
}

func unixTransport(socketPath string) *httpunix.Transport {
	t := &httpunix.Transport{
		DialTimeout:           1 * time.Second,
		RequestTimeout:        5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	t.RegisterLocation("c", socketPath)
	return t
}

func unixClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: unixTransport(socketPath),
	}
}

func RunNode(socketPath string) error {
	c := unixClient(socketPath)
	res, err := c.Get("http+unix://c/upcheck")
	if err != nil {
		return err
	}
	if res.StatusCode == 200 {
		return nil
	}
	return errors.New("Constellation Node API did not respond to upcheck request")
}

type Client struct {
	httpClient *http.Client
}

func (c *Client) doJson(path string, apiReq interface{}) (*http.Response, error) {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(apiReq)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "http+unix://c/"+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err == nil && res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}
	return res, err
}

func (c *Client) SendPayload(pl []byte, b64From string, b64To []string) ([]byte, error) {
	buf := bytes.NewBuffer(pl)
	req, err := http.NewRequest("POST", "http+unix://c/sendraw", buf)
	if err != nil {
		return nil, err
	}
	if b64From != "" {
		req.Header.Set("c11n-from", b64From)
	}
	req.Header.Set("c11n-to", strings.Join(b64To, ","))
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := c.httpClient.Do(req)

	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}

	return ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, res.Body))
}

func (c *Client) SendSignedPayload(signedPayload []byte, b64To []string) ([]byte, error) {
	buf := bytes.NewBuffer(signedPayload)
	req, err := http.NewRequest("POST", "http+unix://c/sendsignedtx", buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("c11n-to", strings.Join(b64To, ","))
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := c.httpClient.Do(req)

	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}

	return ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, res.Body))
}

func (c *Client) ReceivePayload(key []byte) ([]byte, error) {
	req, err := http.NewRequest("GET", "http+unix://c/receiveraw", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("c11n-key", base64.StdEncoding.EncodeToString(key))
	res, err := c.httpClient.Do(req)

	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}

	return ioutil.ReadAll(res.Body)
}

func (c *Client) IsSender(txHash common.EncryptedPayloadHash) (bool, error) {
	req, err := http.NewRequest("GET", "http+unix://c/transaction/" + url.PathEscape(txHash.ToBase64()) +  "/isSender", nil)
	if err != nil {
		return false, err
	}

	res, err := c.httpClient.Do(req)

	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		return false, err
	}

	if res.StatusCode != 200 {
		return false, fmt.Errorf("Non-200 status code: %+v", res)
	}

	out, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, err
	}

	return string(out) == "true", nil
}

func (c *Client) GetParticipants(txHash common.EncryptedPayloadHash) ([]string, error) {
	requestUrl := "http+unix://c/transaction/" + url.PathEscape(txHash.ToBase64()) + "/participants"
	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)

	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}

	out, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	split := strings.Split(string(out), ",")

	return split, nil
}

func NewClient(socketPath string) (*Client, error) {
	return &Client{
		httpClient: unixClient(socketPath),
	}, nil
}
