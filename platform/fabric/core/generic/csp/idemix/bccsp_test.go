/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"crypto/rand"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp/idemix"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Idemix Bridge", func() {

	Describe("setting up the environment with one issuer and one user", func() {
		var (
			CSP             bccsp.BCCSP
			IssuerKey       bccsp.Key
			IssuerPublicKey bccsp.Key
			AttributeNames  []string

			UserKey      bccsp.Key
			NymKey       bccsp.Key
			NymPublicKey bccsp.Key

			IssuerNonce []byte
			credRequest []byte

			credential []byte

			RevocationKey       bccsp.Key
			RevocationPublicKey bccsp.Key
			cri                 []byte
		)

		BeforeEach(func() {
			var err error
			CSP, err = idemix.New(sw.NewDummyKeyStore())
			Expect(err).NotTo(HaveOccurred())

			// Issuer
			AttributeNames = []string{"Attr1", "Attr2", "Attr3", "Attr4", "Attr5"}
			IssuerKey, err = CSP.KeyGen(&csp.IdemixIssuerKeyGenOpts{Temporary: true, AttributeNames: AttributeNames})
			Expect(err).NotTo(HaveOccurred())
			IssuerPublicKey, err = IssuerKey.PublicKey()
			Expect(err).NotTo(HaveOccurred())

			// User
			UserKey, err = CSP.KeyGen(&csp.IdemixUserSecretKeyGenOpts{Temporary: true})
			Expect(err).NotTo(HaveOccurred())

			// User Nym Key
			NymKey, err = CSP.KeyDeriv(UserKey, &csp.IdemixNymKeyDerivationOpts{Temporary: true, IssuerPK: IssuerPublicKey})
			Expect(err).NotTo(HaveOccurred())
			NymPublicKey, err = NymKey.PublicKey()
			Expect(err).NotTo(HaveOccurred())

			IssuerNonce = make([]byte, 32)
			n, err := rand.Read(IssuerNonce)
			Expect(n).To(BeEquivalentTo(32))
			Expect(err).NotTo(HaveOccurred())

			// Credential Request for User
			credRequest, err = CSP.Sign(
				UserKey,
				nil,
				&csp.IdemixCredentialRequestSignerOpts{IssuerPK: IssuerPublicKey, IssuerNonce: IssuerNonce},
			)
			Expect(err).NotTo(HaveOccurred())

			// Credential
			credential, err = CSP.Sign(
				IssuerKey,
				credRequest,
				&csp.IdemixCredentialSignerOpts{
					Attributes: []csp.IdemixAttribute{
						{Type: csp.IdemixBytesAttribute, Value: []byte{0}},
						{Type: csp.IdemixBytesAttribute, Value: []byte{0, 1}},
						{Type: csp.IdemixIntAttribute, Value: 1},
						{Type: csp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
						{Type: csp.IdemixBytesAttribute, Value: []byte{0, 1, 2, 3}},
					},
				},
			)
			Expect(err).NotTo(HaveOccurred())

			// Revocation
			RevocationKey, err = CSP.KeyGen(&csp.IdemixRevocationKeyGenOpts{Temporary: true})
			Expect(err).NotTo(HaveOccurred())
			RevocationPublicKey, err = RevocationKey.PublicKey()
			Expect(err).NotTo(HaveOccurred())

			// CRI
			cri, err = CSP.Sign(
				RevocationKey,
				nil,
				&csp.IdemixCRISignerOpts{},
			)
			Expect(err).NotTo(HaveOccurred())

		})

		It("the environment is properly set", func() {
			// Verify CredRequest
			valid, err := CSP.Verify(
				IssuerPublicKey,
				credRequest,
				nil,
				&csp.IdemixCredentialRequestSignerOpts{IssuerNonce: IssuerNonce},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())

			// Verify Credential
			valid, err = CSP.Verify(
				UserKey,
				credential,
				nil,
				&csp.IdemixCredentialSignerOpts{
					IssuerPK: IssuerPublicKey,
					Attributes: []csp.IdemixAttribute{
						{Type: csp.IdemixBytesAttribute, Value: []byte{0}},
						{Type: csp.IdemixBytesAttribute, Value: []byte{0, 1}},
						{Type: csp.IdemixIntAttribute, Value: 1},
						{Type: csp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
						{Type: csp.IdemixBytesAttribute, Value: []byte{0, 1, 2, 3}},
					},
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())

			// Verify CRI
			valid, err = CSP.Verify(
				RevocationPublicKey,
				cri,
				nil,
				&csp.IdemixCRISignerOpts{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
		})

		Describe("producing an idemix signature with no disclosed attribute", func() {
			var (
				digest    []byte
				signature []byte
			)

			BeforeEach(func() {
				var err error

				digest = []byte("a digest")

				signature, err = CSP.Sign(
					UserKey,
					digest,
					&csp.IdemixSignerOpts{
						Credential: credential,
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: []csp.IdemixAttribute{
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
						},
						RhIndex: 4,
						Epoch:   0,
						CRI:     cri,
					},
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("the signature is valid", func() {
				valid, err := CSP.Verify(
					IssuerPublicKey,
					signature,
					digest,
					&csp.IdemixSignerOpts{
						RevocationPublicKey: RevocationPublicKey,
						Attributes: []csp.IdemixAttribute{
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
						},
						RhIndex: 4,
						Epoch:   0,
					},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})

		})

		Describe("producing an idemix signature with disclosed attributes", func() {
			var (
				digest    []byte
				signature []byte
			)

			BeforeEach(func() {
				var err error

				digest = []byte("a digest")

				signature, err = CSP.Sign(
					UserKey,
					digest,
					&csp.IdemixSignerOpts{
						Credential: credential,
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: []csp.IdemixAttribute{
							{Type: csp.IdemixBytesAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixIntAttribute},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
						},
						RhIndex: 4,
						Epoch:   0,
						CRI:     cri,
					},
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("the signature is valid", func() {
				valid, err := CSP.Verify(
					IssuerPublicKey,
					signature,
					digest,
					&csp.IdemixSignerOpts{
						RevocationPublicKey: RevocationPublicKey,
						Attributes: []csp.IdemixAttribute{
							{Type: csp.IdemixBytesAttribute, Value: []byte{0}},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixIntAttribute, Value: 1},
							{Type: csp.IdemixHiddenAttribute},
							{Type: csp.IdemixHiddenAttribute},
						},
						RhIndex: 4,
						Epoch:   0,
					},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})

		})

		Describe("producing an idemix nym signature", func() {
			var (
				digest    []byte
				signature []byte
			)

			BeforeEach(func() {
				var err error

				digest = []byte("a digest")

				signature, err = CSP.Sign(
					UserKey,
					digest,
					&csp.IdemixNymSignerOpts{
						Nym:      NymKey,
						IssuerPK: IssuerPublicKey,
					},
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("the signature is valid", func() {
				valid, err := CSP.Verify(
					NymPublicKey,
					signature,
					digest,
					&csp.IdemixNymSignerOpts{
						IssuerPK: IssuerPublicKey,
					},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})

		})
	})
})
