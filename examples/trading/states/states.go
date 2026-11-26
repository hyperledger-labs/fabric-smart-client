/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package states holds the ledger states of the trading demo.
package states

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

// Kind is a kind of goods.
type Kind string

const (
	// Gold is mined by the gold producer.
	Gold Kind = "gold"
	// Wood is cut by the wood producer.
	Wood Kind = "wood"
	// Bar is refined from gold.
	Bar Kind = "bar"
	// Plank is milled from wood.
	Plank Kind = "plank"
)

// Role is what a producer may do: produce one raw kind, or refine one raw
// kind into its refined kind.
type Role string

const (
	GoldProducer Role = "gold-producer"
	WoodProducer Role = "wood-producer"
	GoldRefiner  Role = "gold-refiner"
	WoodRefiner  Role = "wood-refiner"
)

// Key prefixes. Each ends in '.', so a range scan from the prefix up to the
// prefix with '.' replaced by '/' (the next byte) covers exactly its keys.
const (
	LotPrefix     = "lot."
	ListingPrefix = "listing."
	balancePrefix = "balance."
	// RegistryID is the key of the one Registry.
	RegistryID = "registry"
)

// LotID is the key of the lot created by transaction txID.
func LotID(txID string) string { return LotPrefix + txID }

// ListingID is the key of the listing created by transaction txID.
func ListingID(txID string) string { return ListingPrefix + txID }

// BalanceID is the key of owner's coin balance.
func BalanceID(owner view.Identity) string { return balancePrefix + owner.UniqueID() }

// Lot is a quantity of one kind of goods, owned by one producer. A lot is
// always sold whole.
type Lot struct {
	ID    string
	Kind  Kind
	Qty   uint64
	Owner view.Identity
}

// SetLinearID sets the lot's ID unless it already has one.
func (l *Lot) SetLinearID(id string) string {
	if len(l.ID) == 0 {
		l.ID = id
	}
	return l.ID
}

// Balance is a producer's coins.
type Balance struct {
	ID    string
	Owner view.Identity
	Coins uint64
}

// SetLinearID sets the balance's ID unless it already has one.
func (b *Balance) SetLinearID(id string) string {
	if len(b.ID) == 0 {
		b.ID = id
	}
	return b.ID
}

// Status says whether a listing can still be bought.
type Status string

const (
	Open Status = "open"
	Sold Status = "sold"
)

// Listing offers a whole lot for sale. Kind and Qty repeat the lot's, so
// buyers can browse listings without reading lots. Price is the agreed price,
// set when the lot is sold.
type Listing struct {
	ID     string
	LotID  string
	Kind   Kind
	Qty    uint64
	Seller view.Identity
	Ask    uint64
	Status Status
	Price  uint64
}

// SetLinearID sets the listing's ID unless it already has one.
func (l *Listing) SetLinearID(id string) string {
	if len(l.ID) == 0 {
		l.ID = id
	}
	return l.ID
}

// Registry maps each producer, by the UniqueID of its identity, to its role.
type Registry struct {
	ID    string
	Roles map[string]Role
}

// SetLinearID sets the registry's ID unless it already has one.
func (r *Registry) SetLinearID(id string) string {
	if len(r.ID) == 0 {
		r.ID = id
	}
	return r.ID
}
