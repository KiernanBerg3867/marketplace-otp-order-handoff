package marketplace

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrOrderNotFound = errors.New("order not found")
	ErrPhoneMismatch = errors.New("phone does not match the order")
	ErrCodeRejected  = errors.New("code was not accepted")
)

type OTPGateway interface {
	SendCode(context.Context, string, string) error
	VerifyCode(context.Context, string, string, string) (bool, error)
}

type Order struct {
	ID            string   `json:"order_id"`
	SellerID      string   `json:"seller_id"`
	BuyerPhone    string   `json:"buyer_phone"`
	SellerAssets  []string `json:"seller_assets"`
	BuyerUpdates  []string `json:"buyer_updates"`
	HandoffStatus string   `json:"handoff_status"`
}

type Handoff struct {
	otp    OTPGateway
	mu     sync.Mutex
	orders map[string]Order
}

func NewHandoff(otp OTPGateway, orders []Order) *Handoff {
	indexed := make(map[string]Order, len(orders))
	for _, order := range orders {
		indexed[order.ID] = order
	}
	return &Handoff{otp: otp, orders: indexed}
}

func (h *Handoff) SendBuyerCode(ctx context.Context, orderID, phone, attemptID string) error {
	h.mu.Lock()
	order, ok := h.orders[orderID]
	h.mu.Unlock()
	if !ok {
		return ErrOrderNotFound
	}
	if order.BuyerPhone != phone {
		return ErrPhoneMismatch
	}
	return h.otp.SendCode(ctx, phone, attemptID+":send")
}

func (h *Handoff) VerifyAndRelease(ctx context.Context, orderID, phone, code, attemptID string) (Order, error) {
	h.mu.Lock()
	order, ok := h.orders[orderID]
	h.mu.Unlock()
	if !ok {
		return Order{}, ErrOrderNotFound
	}
	if order.BuyerPhone != phone {
		return Order{}, ErrPhoneMismatch
	}

	verified, err := h.otp.VerifyCode(ctx, phone, code, attemptID+":verify")
	if err != nil {
		return Order{}, err
	}
	if !verified {
		return Order{}, ErrCodeRejected
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	order = h.orders[orderID]
	order.HandoffStatus = "released_to_buyer"
	order.BuyerUpdates = append(order.BuyerUpdates, "Phone verified; seller assets released")
	h.orders[orderID] = order
	return order, nil
}
