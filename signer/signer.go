// Package signer contains a transaction signer for publish operations.
package signer

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// Signer signs Solana transactions carrying Pyth price updates.
type Signer struct {
	privateKey  solana.PrivateKey
	publicKey   solana.PublicKey
	pythProgram solana.PublicKey
}

// NewSigner loads the unencrypted private key from the provided file.
func NewSigner(privateKeyPath string, pythProgram solana.PublicKey) (*Signer, error) {
	pk, err := solana.PrivateKeyFromSolanaKeygenFile(privateKeyPath)
	if err != nil {
		return nil, err
	}
	return &Signer{
		privateKey:  pk,
		publicKey:   pk.PublicKey(),
		pythProgram: pythProgram,
	}, nil
}

// Pubkey returns the public key of the wallet being managed.
func (s *Signer) Pubkey() solana.PublicKey {
	return s.publicKey
}

// Close should be called when a signer is not used anymore.
func (s *Signer) Close() {
	for i := range s.privateKey {
		s.privateKey[i] = 0
	}
}

// SignPriceUpdate signs Pyth price update operations.
func (s *Signer) SignPriceUpdate(tx *solana.Transaction) error {
	// Verify instructions.
	for _, op := range tx.Message.Instructions {
		/*
			// Find out if signature is requested.
			wantsSig := false
			for _, accIdx := range op.Accounts {
				if accIdx >= uint16(tx.Message.Header.NumRequiredSignatures) {
					continue
				}
				if tx.Message.AccountKeys[accIdx].Equals(s.publicKey) {
					wantsSig = true
					break
				}
			}
			if !wantsSig {
				continue
			}
		*/
		// Reject if requested sig for unknown program instruction.
		requestedProgram := tx.Message.AccountKeys[op.ProgramIDIndex]
		if !requestedProgram.Equals(s.pythProgram) {
			return fmt.Errorf("refusing to sign for program %s", requestedProgram.String())
		}
		// TODO(richard): Restrict to price updates.
	}

	// Actually sign.
	_, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if !key.Equals(s.publicKey) {
			return nil
		}
		return &s.privateKey
	})

	return err
}
