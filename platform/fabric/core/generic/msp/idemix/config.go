package idemix

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/msp"

	im "github.com/IBM/idemix/idemixmsp"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"
)

// SignerConfig contains the crypto material to set up an idemix signing identity
type SignerConfig struct {
	// Cred represents the serialized idemix credential of the default signer
	Cred []byte `protobuf:"bytes,1,opt,name=Cred,proto3" json:"Cred,omitempty"`
	// Sk is the secret key of the default signer, corresponding to credential Cred
	Sk []byte `protobuf:"bytes,2,opt,name=Sk,proto3" json:"Sk,omitempty"`
	// OrganizationalUnitIdentifier defines the organizational unit the default signer is in
	OrganizationalUnitIdentifier string `protobuf:"bytes,3,opt,name=organizational_unit_identifier,json=organizationalUnitIdentifier" json:"organizational_unit_identifier,omitempty"`
	// Role defines whether the default signer is admin, member, peer, or client
	Role int32 `protobuf:"varint,4,opt,name=role,json=role" json:"role,omitempty"`
	// EnrollmentID contains the enrollment id of this signer
	EnrollmentID string `protobuf:"bytes,5,opt,name=enrollment_id,json=enrollmentId" json:"enrollment_id,omitempty"`
	// CRI contains a serialized Credential Revocation Information
	CredentialRevocationInformation []byte `protobuf:"bytes,6,opt,name=credential_revocation_information,json=credentialRevocationInformation,proto3" json:"credential_revocation_information,omitempty"`
	// CurveID specifies the name of the Idemix curve to use, defaults to 'amcl.Fp256bn'
	CurveID string `protobuf:"bytes,7,opt,name=curve_id,json=curveID" json:"curveID,omitempty"`
}

const (
	IdemixConfigDirMsp                  = "msp"
	IdemixConfigDirUser                 = "user"
	IdemixConfigFileIssuerPublicKey     = "IssuerPublicKey"
	IdemixConfigFileRevocationPublicKey = "IssuerRevocationPublicKey"
	IdemixConfigFileSigner              = "SignerConfig"
)

func readFile(file string) ([]byte, error) {
	fileCont, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read file %s", file)
	}

	return fileCont, nil
}

func GetLocalMspConfigWithType(dir string, bccspConfig *factory.FactoryOpts, id string) (*m.MSPConfig, error) {
	mspConfig, err := msp.GetLocalMspConfigWithType(dir, bccspConfig, id, msp.ProviderTypeToString(msp.IDEMIX))
	if err != nil {
		// load it using the fabric-ca format
		mspConfig2, err2 := GetFabricCAIdemixMspConfig(dir, id)
		if err2 != nil {
			return nil, errors.Wrapf(err2, "cannot get idemix msp config from [%s]: [%s]", dir, err)
		}
		mspConfig = mspConfig2
	}
	return mspConfig, nil
}

// GetFabricCAIdemixMspConfig returns the configuration for the Idemix MSP generated by Fabric-CA
func GetFabricCAIdemixMspConfig(dir string, ID string) (*m.MSPConfig, error) {
	ipkBytes, err := readFile(filepath.Join(dir, IdemixConfigFileIssuerPublicKey))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read issuer public key file")
	}

	revocationPkBytes, err := readFile(filepath.Join(dir, IdemixConfigFileRevocationPublicKey))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read revocation public key file")
	}

	idemixConfig := &im.IdemixMSPConfig{
		Name:         ID,
		Ipk:          ipkBytes,
		RevocationPk: revocationPkBytes,
	}

	signerBytes, err := readFile(filepath.Join(dir, IdemixConfigDirUser, IdemixConfigFileSigner))
	if err == nil {
		// signerBytes is a json structure, convert it to protobuf
		si := &SignerConfig{}
		if err := json.Unmarshal(signerBytes, si); err != nil {
			return nil, errors.Wrapf(err, "failed to json unmarshal signer config")
		}

		signerConfig := &im.IdemixMSPSignerConfig{
			Cred:                            si.Cred,
			Sk:                              si.Sk,
			OrganizationalUnitIdentifier:    si.OrganizationalUnitIdentifier,
			Role:                            si.Role,
			EnrollmentId:                    si.EnrollmentID,
			CredentialRevocationInformation: si.CredentialRevocationInformation,
			//RevocationHandle:                si,
		}
		idemixConfig.Signer = signerConfig
	}

	confBytes, err := proto.Marshal(idemixConfig)
	if err != nil {
		return nil, err
	}

	return &m.MSPConfig{Config: confBytes, Type: int32(msp.IDEMIX)}, nil
}
