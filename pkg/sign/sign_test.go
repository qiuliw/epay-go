package sign

import (
	"net/url"
	"testing"
)

func TestVerifyMD5SignIncludesOptionalFields(t *testing.T) {
	key := "7379154dbe2ed797263e3136e7e78437"
	params := url.Values{}
	params.Set("pid", "1")
	params.Set("name", "商品订单号:260811143917555186021512")
	params.Set("type", "alipay")
	params.Set("money", "0.02")
	params.Set("out_trade_no", "260811143917555186021512")
	params.Set("notify_url", "https://www.henduohao.cc/pay/async.260811143917555186021512")
	params.Set("return_url", "https://www.henduohao.cc/pay/sync.260811143917555186021512")
	params.Set("sitename", "260811143917555186021512")
	params.Set("clientip", "212.135.214.6")
	params.Set("device", "pc")
	params.Set("sign", "c0718ea4b2e52ab59f61cb1e56a06e06")
	params.Set("sign_type", "MD5")

	if !VerifyMD5Sign(params, key, params.Get("sign")) {
		t.Fatal("expected full-form signature to verify")
	}

	withoutSitename := url.Values{}
	for k, vs := range params {
		if k == "sitename" {
			continue
		}
		withoutSitename[k] = vs
	}
	if VerifyMD5Sign(withoutSitename, key, params.Get("sign")) {
		t.Fatal("signature must fail when sitename is dropped from verify set")
	}
}
