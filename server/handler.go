package server

import (
	"context"
	"errors"
	"net"
	"sync/atomic"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mitchellh/mapstructure"
	"go.blockdaemon.com/pyth"
	"go.blockdaemon.com/pythian/jsonrpc"
	"go.blockdaemon.com/pythian/schedule"
	"go.uber.org/zap"
)

const (
	rpcErrUnknownSymbol = -32000
	rpcErrNotReady      = -32002
)

type Handler struct {
	*jsonrpc.Mux
	Log       *zap.Logger
	client    *pyth.Client
	buffer    *schedule.Buffer
	publisher solana.PublicKey
	slots     *schedule.SlotMonitor
	subNonce  int64
}

func NewHandler(
	client *pyth.Client,
	updateBuffer *schedule.Buffer,
	publisher solana.PublicKey,
	slots *schedule.SlotMonitor,
) *Handler {
	mux := jsonrpc.NewMux()
	h := &Handler{
		Mux:       mux,
		Log:       zap.NewNop(),
		client:    client,
		buffer:    updateBuffer,
		publisher: publisher,
		slots:     slots,
		subNonce:  1,
	}
	mux.HandleFunc("get_product_list", h.handleGetProductList)
	mux.HandleFunc("get_product", h.handleGetProduct)
	mux.HandleFunc("get_all_products", h.handleGetAllProducts)
	mux.HandleFunc("update_price", h.handleUpdatePrice)
	mux.HandleFunc("subscribe_price", h.handleSubscribePrice)
	return h
}

func (h *Handler) getAllProductsAndPrices(ctx context.Context) ([]pyth.ProductAccountEntry, map[solana.PublicKey][]pyth.PriceAccountEntry, error) {
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

func (h *Handler) handleGetProductList(ctx context.Context, req jsonrpc.Request, _ jsonrpc.Requester) *jsonrpc.Response {
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

func (h *Handler) handleGetAllProducts(ctx context.Context, req jsonrpc.Request, _ jsonrpc.Requester) *jsonrpc.Response {
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

func (h *Handler) handleGetProduct(ctx context.Context, req jsonrpc.Request, _ jsonrpc.Requester) *jsonrpc.Response {
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

func (h *Handler) handleUpdatePrice(_ context.Context, req jsonrpc.Request, _ jsonrpc.Requester) *jsonrpc.Response {
	// Decode params.
	var params struct {
		Account solana.PublicKey `json:"account"`
		Price   int64            `json:"price"`
		Conf    uint64           `json:"conf"`
		Status  string           `json:"status"`
	}
	if err := mapstructure.Decode(req.Params, &params); err != nil {
		return jsonrpc.NewInvalidParamsResponse(req.ID)
	}
	if params.Account.IsZero() || params.Price == 0 || params.Conf == 0 || params.Status == "" {
		return jsonrpc.NewInvalidParamsResponse(req.ID)
	}

	// Assemble instruction.
	update := pyth.CommandUpdPrice{
		Status:  statusFromString(params.Status),
		Price:   params.Price,
		Conf:    params.Conf,
		PubSlot: h.slots.Slot(),
	}
	ins := pyth.NewInstructionBuilder(h.client.Env.Program).
		UpdPrice(h.publisher, params.Account, update).(*pyth.Instruction)

	// Push instruction to write buffer. (Will be picked up by scheduler)
	h.buffer.PushUpdate(ins)

	return jsonrpc.NewResultResponse(req.ID, 0)
}

func (h *Handler) handleSubscribePrice(_ context.Context, req jsonrpc.Request, callback jsonrpc.Requester) *jsonrpc.Response {
	if req.ID == nil {
		return nil
	}

	// Decode params.
	var params struct {
		Account solana.PublicKey `json:"account"`
	}
	if err := mapstructure.Decode(req.Params, &params); err != nil {
		return jsonrpc.NewInvalidParamsResponse(req.ID)
	}
	if params.Account.IsZero() {
		return jsonrpc.NewInvalidParamsResponse(req.ID)
	}

	// Launch new subscription worker.
	go h.asyncSubscribePrice(params.Account, callback)
	return newSubscriptionResponse(req.ID, h.newSubID())
}

func (h *Handler) asyncSubscribePrice(account solana.PublicKey, callback jsonrpc.Requester) {
	// TODO(richard): This is inefficient, no need to stream copy of all price updates for _each_ subscription.
	stream := h.client.StreamPriceAccounts()
	defer stream.Close()

	handler := pyth.NewPriceEventHandler(stream)
	handler.OnPriceChange(account, func(update pyth.PriceUpdate) {
		err := callback.AsyncRequestJSONRPC(context.Background(), "notify_price", priceUpdate{
			Price:     update.CurrentInfo.Price,
			Conf:      update.CurrentInfo.Conf,
			Status:    statusToString(update.CurrentInfo.Status),
			ValidSlot: update.CurrentInfo.PubSlot,
			PubSlot:   update.CurrentInfo.PubSlot,
		})
		if errors.Is(err, net.ErrClosed) {
			stream.Close()
		} else if err != nil {
			h.Log.Warn("Failed to deliver async price update", zap.Error(err))
		}
	})
}

func newSubscriptionResponse(reqID interface{}, subID int64) *jsonrpc.Response {
	var result struct {
		Subscription int64 `json:"subscription"`
	}
	result.Subscription = subID
	return jsonrpc.NewResultResponse(reqID, &result)
}

func (h *Handler) newSubID() int64 {
	return atomic.AddInt64(&h.subNonce, 1)
}
