// internal/payment/xunhupay.go
package payment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const defaultXunhuGateway = "https://api.xunhupay.com/payment/do.html"
const defaultXunhuQuery = "https://api.xunhupay.com/payment/query.html"

// XunhuConfig 虎皮椒通道配置
type XunhuConfig struct {
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	Gateway     string `json:"gateway"`      // 支付网关，默认官方 do.html
	QueryURL    string `json:"query_url"`    // 查询网关，默认官方 query.html
	WapName     string `json:"wap_name"`     // 店铺名
	Plugins     string `json:"plugins"`      // 识别对接方
	PreferQR    bool   `json:"prefer_qr"`    // true 时优先返回 url_qrcode
	PaymentType string `json:"payment_type"` // 可选：alipay / wechat（部分账户支持）
}

// XunhuAdapter 虎皮椒适配器
type XunhuAdapter struct {
	config *XunhuConfig
	client *http.Client
}

func newXunhuAdapter(configJSON json.RawMessage) (PaymentAdapter, error) {
	var m map[string]interface{}
	_ = json.Unmarshal(configJSON, &m)
	preferQR := false
	if m != nil {
		if _, ok := m["app_id"]; !ok {
			if v, ok2 := m["appid"]; ok2 {
				m["app_id"] = v
			}
		}
		if _, ok := m["app_secret"]; !ok {
			if v, ok2 := m["appsecret"]; ok2 {
				m["app_secret"] = v
			} else if v, ok2 := m["secret"]; ok2 {
				m["app_secret"] = v
			}
		}
		if v, ok := m["prefer_qr"]; ok {
			switch t := v.(type) {
			case bool:
				preferQR = t
			case string:
				preferQR = t == "true" || t == "1" || strings.EqualFold(t, "yes")
			case float64:
				preferQR = t != 0
			}
			delete(m, "prefer_qr")
		}
		b, _ := json.Marshal(m)
		configJSON = b
	}

	var cfg XunhuConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, err
	}
	cfg.PreferQR = preferQR
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.AppSecret) == "" {
		return nil, errors.New("xunhupay app_id/app_secret required")
	}
	if strings.TrimSpace(cfg.Gateway) == "" {
		cfg.Gateway = defaultXunhuGateway
	}
	if strings.TrimSpace(cfg.QueryURL) == "" {
		cfg.QueryURL = defaultXunhuQuery
	}
	if strings.TrimSpace(cfg.WapName) == "" {
		cfg.WapName = "AdminCloud"
	}
	if strings.TrimSpace(cfg.Plugins) == "" {
		cfg.Plugins = "epay-go"
	}

	return &XunhuAdapter{
		config: &cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// NewXunhuAlipayAdapter 虎皮椒-支付宝
func NewXunhuAlipayAdapter(configJSON json.RawMessage) (PaymentAdapter, error) {
	return newXunhuAdapter(configJSON)
}

// NewXunhuWechatAdapter 虎皮椒-微信
func NewXunhuWechatAdapter(configJSON json.RawMessage) (PaymentAdapter, error) {
	return newXunhuAdapter(configJSON)
}

func (a *XunhuAdapter) sign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "hash" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	b.WriteString(a.config.AppSecret)
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (a *XunhuAdapter) postForm(endpoint string, params map[string]string) (map[string]interface{}, error) {
	params["appid"] = a.config.AppID
	params["time"] = strconv.FormatInt(time.Now().Unix(), 10)
	if params["nonce_str"] == "" {
		params["nonce_str"] = params["time"]
	}
	params["hash"] = a.sign(params)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	resp, err := a.client.PostForm(endpoint, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("invalid xunhupay response: %s", string(body))
	}
	return out, nil
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func (a *XunhuAdapter) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*CreateOrderResponse, error) {
	_ = ctx
	params := map[string]string{
		"version":        "1.1",
		"trade_order_id": req.TradeNo,
		"total_fee":      req.Amount.StringFixed(2),
		"title":          req.Subject,
		"notify_url":     req.NotifyURL,
		"return_url":     req.ReturnURL,
		"callback_url":   req.ReturnURL,
		"wap_name":       a.config.WapName,
		"plugins":        a.config.Plugins,
	}
	if a.config.PaymentType != "" {
		params["type"] = a.config.PaymentType
	}

	out, err := a.postForm(a.config.Gateway, params)
	if err != nil {
		return nil, err
	}

	errcode := asString(out["errcode"])
	if errcode != "" && errcode != "0" {
		msg := asString(out["errmsg"])
		if msg == "" {
			msg = "xunhupay create order failed"
		}
		return nil, errors.New(msg)
	}

	payURL := asString(out["url"])
	qrURL := asString(out["url_qrcode"])

	method := strings.ToLower(strings.TrimSpace(req.PayMethod))
	useQR := a.config.PreferQR || method == "scan" || method == "qrcode" || method == "native"
	if useQR && qrURL != "" {
		return &CreateOrderResponse{PayType: "qrcode", PayURL: qrURL}, nil
	}
	if payURL != "" {
		return &CreateOrderResponse{PayType: "redirect", PayURL: payURL}, nil
	}
	if qrURL != "" {
		return &CreateOrderResponse{PayType: "qrcode", PayURL: qrURL}, nil
	}
	return nil, errors.New("xunhupay returned empty pay url")
}

func (a *XunhuAdapter) QueryOrder(ctx context.Context, tradeNo string) (*QueryOrderResponse, error) {
	_ = ctx
	out, err := a.postForm(a.config.QueryURL, map[string]string{
		"out_trade_order": tradeNo,
	})
	if err != nil {
		return nil, err
	}
	errcode := asString(out["errcode"])
	if errcode != "" && errcode != "0" {
		msg := asString(out["errmsg"])
		if msg == "" {
			msg = "xunhupay query failed"
		}
		return nil, errors.New(msg)
	}

	statusRaw := strings.ToUpper(asString(out["status"]))
	status := "pending"
	switch statusRaw {
	case "OD", "PAID", "SUCCESS":
		status = "paid"
	case "CD", "CLOSED", "CANCEL":
		status = "closed"
	case "RD", "REFUNDED":
		status = "refunded"
	}

	amount, _ := decimal.NewFromString(asString(out["total_fee"]))
	apiTradeNo := asString(out["transaction_id"])
	if apiTradeNo == "" {
		apiTradeNo = asString(out["open_order_id"])
	}

	return &QueryOrderResponse{
		TradeNo:    tradeNo,
		ApiTradeNo: apiTradeNo,
		Amount:     amount,
		Status:     status,
	}, nil
}

func (a *XunhuAdapter) Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	_ = ctx
	return &RefundResponse{
		RefundNo:     req.RefundNo,
		Status:       "failed",
		ErrorMessage: "xunhupay refund is not supported via this adapter",
	}, nil
}

func (a *XunhuAdapter) ParseNotify(ctx context.Context, r *http.Request) (*NotifyResult, error) {
	_ = ctx
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	params := map[string]string{}
	for k, vs := range r.Form {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}

	got := params["hash"]
	if got == "" {
		return nil, errors.New("missing hash")
	}
	if !strings.EqualFold(a.sign(params), got) {
		return nil, errors.New("xunhupay notify signature mismatch")
	}

	statusRaw := strings.ToUpper(params["status"])
	status := "fail"
	if statusRaw == "OD" {
		status = "success"
	}

	amount, _ := decimal.NewFromString(params["total_fee"])
	apiTradeNo := params["transaction_id"]
	if apiTradeNo == "" {
		apiTradeNo = params["open_order_id"]
	}

	return &NotifyResult{
		TradeNo:    params["trade_order_id"],
		ApiTradeNo: apiTradeNo,
		Amount:     amount,
		Buyer:      params["open_order_id"],
		Status:     status,
	}, nil
}

func (a *XunhuAdapter) NotifySuccess() string {
	return "success"
}

func init() {
	Register("xh-alipay", NewXunhuAlipayAdapter)
	Register("xh-wxpay", NewXunhuWechatAdapter)
}
