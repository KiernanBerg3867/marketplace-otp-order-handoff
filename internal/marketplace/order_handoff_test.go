package marketplace

import (
	"context"
	"errors"
	"testing"
)

type fakeOTP struct {
	verified bool
	err      error
}

func (f fakeOTP) SendCode(context.Context, string, string) error { return f.err }
func (f fakeOTP) VerifyCode(context.Context, string, string, string) (bool, error) {
	return f.verified, f.err
}

func TestVerificationControlsOrderHandoff(t *testing.T) {
	tests := []struct {
		name       string
		phone      string
		otp        fakeOTP
		wantStatus string
		wantErr    error
	}{
		{name: "accepted code releases seller assets", phone: "+14155550100", otp: fakeOTP{verified: true}, wantStatus: "released_to_buyer"},
		{name: "rejected code releases nothing", phone: "+14155550100", otp: fakeOTP{verified: false}, wantErr: ErrCodeRejected},
		{name: "different buyer cannot claim order", phone: "+14155550199", otp: fakeOTP{verified: true}, wantErr: ErrPhoneMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handoff := NewHandoff(tt.otp, []Order{{
				ID: "order-42", SellerID: "seller-7", BuyerPhone: "+14155550100",
				SellerAssets: []string{"condition-report.pdf", "pickup-photo.jpg"},
				BuyerUpdates: []string{"Seller packed the order"}, HandoffStatus: "awaiting_phone_verification",
			}})

			order, err := handoff.VerifyAndRelease(context.Background(), "order-42", tt.phone, "123456", "login-42")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if order.HandoffStatus != "" || len(order.SellerAssets) != 0 {
					t.Fatal("rejected handoff exposed order data")
				}
				return
			}
			if order.HandoffStatus != tt.wantStatus || len(order.SellerAssets) != 2 || len(order.BuyerUpdates) != 2 {
				t.Fatalf("released order = %+v", order)
			}
		})
	}
}
