package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"mime"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"

	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
)

// connectTimeout bounds dialing, TLS, LOGIN and SELECT together.
const connectTimeout = 20 * time.Second

// Fixed servers of the presets. Both use implicit TLS on 993.
var presetHosts = map[api.MailProvider]string{
	api.Gmail:  "imap.gmail.com",
	api.Aliyun: "imap.qiye.aliyun.com",
}

// imapConfig is what a connection needs.
type imapConfig struct {
	provider api.MailProvider
	host     string
	port     int
	username string
	password string
}

// connError is a connection failure with a message for people. auth is true
// when the server said the user name or password is wrong; those are not
// retried automatically.
type connError struct {
	msg  string
	auth bool
	err  error
}

func (e *connError) Error() string { return e.msg }
func (e *connError) Unwrap() error { return e.err }

func isAuthError(err error) bool {
	var ce *connError
	return errors.As(err, &ce) && ce.auth
}

// wordDecoder decodes RFC 2047 headers in every charset go-message knows,
// including GBK and GB2312.
var wordDecoder = &mime.WordDecoder{CharsetReader: charset.Reader}

// connect dials host:port with TLS, logs in and selects INBOX. handler gets
// unilateral data such as new mail during IDLE.
func (m *Module) connect(ctx context.Context, c imapConfig, handler *imapclient.UnilateralDataHandler) (*imapclient.Client, *imap.SelectData, error) {
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	tlsCfg := &tls.Config{}
	if m.tlsConfig != nil {
		tlsCfg = m.tlsConfig.Clone()
	}
	tlsCfg.ServerName = c.host
	opts := &imapclient.Options{
		TLSConfig:             tlsCfg,
		WordDecoder:           wordDecoder,
		UnilateralDataHandler: handler,
		Dialer:                &net.Dialer{Timeout: connectTimeout},
	}

	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	type result struct {
		client *imapclient.Client
		sel    *imap.SelectData
		err    error
	}
	done := make(chan result, 1)
	clientCh := make(chan *imapclient.Client, 1)
	go func() {
		cl, err := imapclient.DialTLS(addr, opts)
		if err != nil {
			done <- result{err: dialError(addr, err)}
			return
		}
		clientCh <- cl
		if ctx.Err() != nil {
			cl.Close()
			done <- result{err: ctx.Err()}
			return
		}
		fail := func(err error) {
			cl.Close()
			done <- result{err: err}
		}
		if err := cl.WaitGreeting(); err != nil {
			fail(&connError{msg: "服务器没有正常应答：" + err.Error(), err: err})
			return
		}
		if err := cl.Login(c.username, c.password).Wait(); err != nil {
			fail(loginError(c.provider, err))
			return
		}
		sel, err := cl.Select("INBOX", nil).Wait()
		if err != nil {
			fail(&connError{msg: "打不开收件箱：" + err.Error(), err: err})
			return
		}
		done <- result{client: cl, sel: sel}
	}()
	select {
	case r := <-done:
		return r.client, r.sel, r.err
	case <-ctx.Done():
		select {
		case cl := <-clientCh:
			cl.Close() // 让还在等应答的命令马上失败
		default:
		}
		if r := <-done; r.client != nil {
			r.client.Close()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, nil, &connError{msg: "连接邮箱超时（20 秒没连上）", err: ctx.Err()}
		}
		return nil, nil, ctx.Err()
	}
}

func dialError(addr string, err error) error {
	var (
		recErr  tls.RecordHeaderError
		certErr *tls.CertificateVerificationError
		unkErr  x509.UnknownAuthorityError
		hostErr x509.HostnameError
	)
	if errors.As(err, &recErr) || errors.As(err, &certErr) || errors.As(err, &unkErr) || errors.As(err, &hostErr) ||
		strings.Contains(err.Error(), "tls:") {
		return &connError{msg: "TLS 握手失败（" + addr + "）：" + err.Error() + "。端口要用 SSL 的那个，一般是 993", err: err}
	}
	return &connError{msg: "连不上服务器 " + addr + "：" + err.Error(), err: err}
}

func loginError(p api.MailProvider, err error) error {
	text := err.Error()
	low := strings.ToLower(text)
	if strings.Contains(low, "not enabled for imap") || strings.Contains(low, "imap is disabled") ||
		strings.Contains(low, "imap access is disabled") || strings.Contains(text, "未开启") || strings.Contains(text, "没有开启") {
		return &connError{msg: "邮箱没有开 IMAP，请先在邮箱设置里打开 IMAP", auth: true, err: err}
	}
	var ierr *imap.Error
	if errors.As(err, &ierr) && ierr.Type == imap.StatusResponseTypeNo {
		msg := "用户名或密码不对"
		if p == api.Gmail {
			msg += "。Gmail 要用应用专用密码，不是登录密码"
		}
		if ierr.Text != "" {
			msg += fmt.Sprintf("（服务器说：%s）", ierr.Text)
		}
		return &connError{msg: msg, auth: true, err: err}
	}
	return &connError{msg: "登录失败：" + text, err: err}
}
