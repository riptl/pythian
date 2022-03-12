package server

import (
	"context"
	"errors"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mitchellh/mapstructure"
	"go.blockdaemon.com/pyth"
	"go.blockdaemon.com/pythian/jsonrpc"
	"go.blockdaemon.com/pythian/schedule"
	"go.blockdaemon.com/pythian/signer"
)

const (
	rpcErrUnknownSymbol = -32000
	rpcErrNotReady      = -32002
)

type Handler struct {
	*jsonrpc.Mux
	client    *pyth.Client
	signer    *signer.Signer
	blockhash *schedule.BlockHashMonitor
}

func NewHandler(client *pyth.Client, signer *signer.Signer, blockhash *schedule.BlockHashMonitor) *Handler {
	mux := jsonrpc.NewMux()
	h := &Handler{
		Mux:       mux,
		client:    client,
		signer:    signer,
		blockhash: blockhash,
	}
	mux.HandleFunc("get_product_list", h.handleGetProductList)
	mux.HandleFunc("get_product", h.handleGetProduct)
	mux.HandleFunc("get_all_products", h.handleGetAllProducts)
	mux.HandleFunc("update_price", h.handleUpdatePrice)
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

	// TODO(richard): Defer updates and publish according to schedule.
	tx, err := solana.NewTransactionBuilder().
		AddInstruction(
			pyth.NewInstructionBuilder(h.client.Env.Program).
				UpdPrice(h.signer.Pubkey(), params.Account, pyth.CommandUpdPrice{
					Status:  statusFromString(params.Status),
					Price:   params.Price,
					Conf:    params.Conf,
					PubSlot: 0, // TODO
				}),
		).
		SetRecentBlockHash(h.blockhash.GetRecentBlockHash().Blockhash).
		Build()
	if err != nil {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrNotReady, "failed to assemble update_price tx: "+err.Error())
	}

	if err := h.signer.SignPriceUpdate(tx); err != nil {
		return jsonrpc.NewErrorStringResponse(req.ID, rpcErrNotReady, "failed to sign update_price tx: "+err.Error())
	}

	return jsonrpc.NewResultResponse(req.ID, 0)
}
