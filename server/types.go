package server

import "go.blockdaemon.com/pyth"

type productAccount struct {
	Account  string            `json:"account"`
	AttrDict map[string]string `json:"attr_dict"`
	Prices   []priceAccount    `json:"price"`
}

type priceAccount struct {
	Account       string `json:"account"`
	PriceExponent int    `json:"price_exponent"`
	PriceType     string `json:"price_type"`
}

type productAccountDetail struct {
	Account       string               `json:"account"`
	AttrDict      map[string]string    `json:"attr_dict"`
	PriceAccounts []priceAccountDetail `json:"price_accounts"`
}

type priceAccountDetail struct {
	Account           string             `json:"account"`
	PriceType         string             `json:"price_type"`
	PriceExponent     int                `json:"price_exponent"`
	Status            string             `json:"status"`
	Price             int64              `json:"price"`
	Conf              int64              `json:"conf"`
	EmaPrice          int64              `json:"ema_price"`
	EmaConfidence     int64              `json:"ema_confidence"`
	ValidSlot         uint64             `json:"valid_slot"`
	PubSlot           uint64             `json:"pub_slot"`
	PrevSlot          uint64             `json:"prev_slot"`
	PrevPrice         int64              `json:"prev_price"`
	PrevConf          int64              `json:"prev_conf"`
	PublisherAccounts []publisherAccount `json:"publisher_accounts"`
}

type publisherAccount struct {
	Account string `json:"account"`
	Status  string `json:"status"`
	Price   int64  `json:"price"`
	Conf    int64  `json:"conf"`
	Slot    uint64 `json:"slot"`
}

type subscriptionUpdate struct {
	Result       interface{} `json:"result"`
	Subscription uint64      `json:"subscription"`
}

type priceUpdate struct {
	Price     int64  `json:"price"`
	Conf      uint64 `json:"conf"`
	Status    string `json:"status"`
	ValidSlot uint64 `json:"valid_slot"`
	PubSlot   uint64 `json:"pub_slot"`
}

func productToJSON(product pyth.ProductAccountEntry, prices []pyth.PriceAccountEntry) productAccount {
	acc := productAccount{
		Account:  product.Pubkey.String(),
		AttrDict: product.Attrs.KVs(),
		Prices:   make([]priceAccount, len(prices)),
	}
	for i, price := range prices {
		acc.Prices[i] = priceToJSON(price)
	}
	return acc
}

func priceToJSON(price pyth.PriceAccountEntry) priceAccount {
	return priceAccount{
		Account:       price.Pubkey.String(),
		PriceExponent: int(price.Exponent),
		PriceType:     priceTypeToString(price.PriceType),
	}
}

func productToDetailJSON(product pyth.ProductAccountEntry, prices []pyth.PriceAccountEntry) productAccountDetail {
	acc := productAccountDetail{
		Account:       product.Pubkey.String(),
		AttrDict:      product.Attrs.KVs(),
		PriceAccounts: make([]priceAccountDetail, len(prices)),
	}
	for i, price := range prices {
		acc.PriceAccounts[i] = priceToDetailJSON(price)
	}
	return acc
}

func priceToDetailJSON(price pyth.PriceAccountEntry) priceAccountDetail {
	acc := priceAccountDetail{
		Account:       price.Pubkey.String(),
		PriceType:     priceTypeToString(price.PriceType),
		PriceExponent: int(price.Exponent),
		Status:        statusToString(price.Agg.Status),
		Price:         price.Agg.Price,
		Conf:          int64(price.Agg.Conf),
		EmaPrice:      price.Twap.Val,
		EmaConfidence: price.Twac.Val,
		ValidSlot:     price.ValidSlot,
		PubSlot:       price.Agg.PubSlot,
		PrevSlot:      price.PrevSlot,
		PrevPrice:     price.PrevPrice,
		PrevConf:      int64(price.PrevConf),
	}
	publishers := make([]publisherAccount, 0, len(price.Components))
	for _, comp := range price.Components {
		if comp.Publisher.IsZero() {
			continue
		}
		publishers = append(publishers, publisherAccount{
			Account: comp.Publisher.String(),
			Status:  statusToString(comp.Latest.Status),
			Price:   comp.Latest.Price,
			Conf:    int64(comp.Latest.Conf),
			Slot:    comp.Latest.PubSlot,
		})
	}
	acc.PublisherAccounts = publishers
	return acc
}

func priceTypeToString(priceType uint32) string {
	switch priceType {
	case 1:
		return "price"
	default:
		return "unknown"
	}
}

func statusToString(status uint32) string {
	switch status {
	case pyth.PriceStatusTrading:
		return "trading"
	case pyth.PriceStatusAuction:
		return "auction"
	case pyth.PriceStatusHalted:
		return "halted"
	default:
		return "unknown"
	}
}

func statusFromString(status string) uint32 {
	switch status {
	case "trading":
		return pyth.PriceStatusTrading
	case "auction":
		return pyth.PriceStatusAuction
	case "halted":
		return pyth.PriceStatusHalted
	default:
		return pyth.PriceStatusUnknown
	}
}
