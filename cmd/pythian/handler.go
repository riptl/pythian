package main

import (
	"context"
	"errors"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mitchellh/mapstructure"
	"go.blockdaemon.com/pyth"
	"go.blockdaemon.com/pythian/pkg/jsonrpc"
)

const (
	rpcErrUnknownSymbol = -32000
	rpcErrNotReady      = -32002
)

type rpcHandler struct {
	*jsonrpc.Mux
	client *pyth.Client
}

func newRPCHandler(client *pyth.Client) *rpcHandler {
	mux := jsonrpc.NewMux()
	h := &rpcHandler{
		Mux:    mux,
		client: client,
	}
	mux.HandleFunc("get_product_list", h.handleGetProductList)
	mux.HandleFunc("get_product", h.handleGetProduct)
	mux.HandleFunc("get_all_products", h.handleGetAllProducts)
	return h
}

func (h *rpcHandler) getAllProductsAndPrices(ctx context.Context) ([]pyth.ProductAccountEntry, map[solana.PublicKey][]pyth.PriceAccountEntry, error) {
	products, err := h.client.GetAllProductAccounts(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, nil, err
	}
	priceKeys := make([]solana.PublicKey, 0, len(products))
	for _, product := range products {
		if !product.FirstPrice.IsZero() {
			priceKeys = append(priceKeys, product.FirstPrice)
		}
	}
	prices, err := h.client.GetPriceAccountsRecursive(ctx, rpc.CommitmentConfirmed, priceKeys...)
	if err != nil {
		return nil, nil, err
	}
	pricesPerProduct := make(map[solana.PublicKey][]pyth.PriceAccountEntry)
	for _, price := range prices {
		pricesPerProduct[price.Product] = append(pricesPerProduct[price.Product], price)
	}
	return products, pricesPerProduct, nil
}

func (h *rpcHandler) handleGetProductList(ctx context.Context, req jsonrpc.Request, _ jsonrpc.Requester) *jsonrpc.Response {
	products, pricesPerProduct, err := h.getAllProductsAndPrices(ctx)
	if err != nil {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrNotReady, "failed to get products: "+err.Error())
	}
	products2 := make([]productAccount, len(products))
	for i, prod := range products {
		products2[i] = productToJSON(prod, pricesPerProduct[prod.Pubkey])
	}
	return jsonrpc.NewResultResponse(req.ID, products2)
}

func (h *rpcHandler) handleGetAllProducts(ctx context.Context, req jsonrpc.Request, _ jsonrpc.Requester) *jsonrpc.Response {
	products, pricesPerProduct, err := h.getAllProductsAndPrices(ctx)
	if err != nil {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrNotReady, "failed to get products: "+err.Error())
	}
	products2 := make([]productAccountDetail, len(products))
	for i, prod := range products {
		products2[i] = productToDetailJSON(prod, pricesPerProduct[prod.Pubkey])
	}
	return jsonrpc.NewResultResponse(req.ID, products2)
}

func (h *rpcHandler) handleGetProduct(ctx context.Context, req jsonrpc.Request, _ jsonrpc.Requester) *jsonrpc.Response {
	// Decode params.
	var params struct {
		Account solana.PublicKey `json:"account"`
	}
	if err := mapstructure.Decode(req.Params, &params); err != nil {
		return jsonrpc.NewInvalidParamsResponse(req.ID)
	}

	// Retrieve data from chain.
	entry, err := h.client.GetProductAccount(ctx, params.Account, rpc.CommitmentConfirmed)
	if errors.Is(err, rpc.ErrNotFound) {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrUnknownSymbol, "unknown symbol")
	} else if err != nil {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrNotReady, "failed to get product: "+err.Error())
	}
	prices, err := h.client.GetPriceAccountsRecursive(ctx, rpc.CommitmentConfirmed, entry.FirstPrice)
	if errors.Is(err, rpc.ErrNotFound) {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrUnknownSymbol, "unknown symbol")
	} else if err != nil {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrNotReady, "failed to get price accs: "+err.Error())
	}

	return jsonrpc.NewResultResponse(req.ID, productToDetailJSON(entry, prices))
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
		PriceType:     priceTypeString(price.PriceType),
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
		PriceType:     priceTypeString(price.PriceType),
		PriceExponent: int(price.Exponent),
		Status:        statusString(price.Agg.Status),
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
			Status:  statusString(comp.Latest.Status),
			Price:   comp.Latest.Price,
			Conf:    int64(comp.Latest.Conf),
			Slot:    comp.Latest.PubSlot,
		})
	}
	acc.PublisherAccounts = publishers
	return acc
}

func priceTypeString(priceType uint32) string {
	switch priceType {
	case 1:
		return "price"
	default:
		return "unknown"
	}
}

func statusString(status uint32) string {
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
