package vault

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pactus-project/pactus/crypto"
	"github.com/pactus-project/pactus/crypto/bls"
	"github.com/pactus-project/pactus/crypto/bls/hdkeychain"
	"github.com/pactus-project/pactus/crypto/ed25519"
	"github.com/pactus-project/pactus/util/testsuite"
	"github.com/pactus-project/pactus/wallet/addresspath"
	"github.com/pactus-project/pactus/wallet/encrypter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tPassword = "super_secret_password"

type testData struct {
	*testsuite.TestSuite

	vault       *Vault
	mnemonic    string
	importedPrv crypto.PrivateKey
}

// setup returns an instances of vault fo testing.
func setup(t *testing.T) *testData {
	t.Helper()

	ts := testsuite.NewTestSuite(t)

	mnemonic, _ := GenerateMnemonic(128)
	_, importedPrv := ts.RandBLSKeyPair()
	vault, err := CreateVaultFromMnemonic(mnemonic, 21888)
	assert.NoError(t, err)

	key, _ := hdkeychain.NewKeyFromString(vault.Purposes.PurposeBLS.XPubAccount)
	assert.False(t, key.IsPrivate())

	// Create some test address
	_, err = vault.NewBLSAccountAddress("bls-account-address")
	assert.NoError(t, err)
	_, err = vault.NewEd25519AccountAddress("ed25519-account-address", "")
	assert.NoError(t, err)
	_, err = vault.NewValidatorAddress("validator-address")
	assert.NoError(t, err)

	assert.NoError(t, vault.ImportBLSPrivateKey("", importedPrv))
	assert.False(t, vault.IsEncrypted())

	opts := []encrypter.Option{
		encrypter.OptionIteration(1),
		encrypter.OptionMemory(1),
		encrypter.OptionParallelism(1),
	}

	err = vault.UpdatePassword("", tPassword, opts...)
	assert.NoError(t, err)
	assert.True(t, vault.IsEncrypted())

	return &testData{
		TestSuite:   ts,
		vault:       vault,
		mnemonic:    mnemonic,
		importedPrv: importedPrv,
	}
}

func TestAddressInfo(t *testing.T) {
	td := setup(t)

	assert.Equal(t, td.vault.AddressCount(), 5)
	infos := td.vault.AddressInfos()
	for _, i := range infos {
		info := td.vault.AddressInfo(i.Address)

		addr, _ := crypto.AddressFromString(info.Address)
		path, _ := addresspath.FromString(info.Path)

		switch path.Purpose() {
		case H(PurposeBLS12381):
			if addr.IsValidatorAddress() {
				assert.Equal(t, fmt.Sprintf("m/12381'/%d'/1'/%d",
					td.vault.CoinType, path.AddressIndex()), info.Path)
			}

			if addr.IsAccountAddress() {
				assert.Equal(t, fmt.Sprintf("m/12381'/%d'/2'/%d",
					td.vault.CoinType, path.AddressIndex()), info.Path)
			}
		case H(PurposeBIP44):
			assert.Equal(t, fmt.Sprintf("m/44'/%d'/3'/%d'",
				td.vault.CoinType, path.AddressIndex()-addresspath.HardenedKeyStart), info.Path)

		case H(PurposeImportPrivateKey):
			if addr.IsValidatorAddress() {
				assert.Equal(t, fmt.Sprintf("m/65535'/%d'/1'/%d'",
					td.vault.CoinType, path.AddressIndex()-hdkeychain.HardenedKeyStart), info.Path)
			}

			if addr.IsAccountAddress() {
				assert.Equal(t, fmt.Sprintf("m/65535'/%d'/2'/%d'",
					td.vault.CoinType, path.AddressIndex()-hdkeychain.HardenedKeyStart), info.Path)
			}

		default:
			assert.Fail(t, "not supported")
		}
	}

	// Neutered
	neutered := td.vault.Neuter()
	assert.Equal(t, 5, neutered.AddressCount())
}

func TestSortAddressInfo(t *testing.T) {
	td := setup(t)

	infos := td.vault.AddressInfos()

	assert.Equal(t, "m/44'/21888'/3'/0'", infos[0].Path)
	assert.Equal(t, "m/12381'/21888'/1'/0", infos[1].Path)
	assert.Equal(t, "m/12381'/21888'/2'/0", infos[2].Path)
	assert.Equal(t, "m/65535'/21888'/1'/0'", infos[3].Path)
	assert.Equal(t, "m/65535'/21888'/2'/0'", infos[4].Path)
}

func TestAllAccountAddresses(t *testing.T) {
	td := setup(t)

	accountAddrs := td.vault.AllAccountAddresses()
	for _, i := range accountAddrs {
		path, err := addresspath.FromString(i.Path)
		assert.NoError(t, err)

		assert.NotEqual(t, H(crypto.AddressTypeValidator), path.AddressType())
	}
}

func TestAllValidatorAddresses(t *testing.T) {
	td := setup(t)

	validatorAddrs := td.vault.AllValidatorAddresses()
	for _, i := range validatorAddrs {
		info := td.vault.AddressInfo(i.Address)
		assert.Equal(t, i.Address, info.Address)

		path, _ := addresspath.FromString(info.Path)

		switch path.Purpose() {
		case H(PurposeBLS12381):
			assert.Equal(t, fmt.Sprintf("m/%d'/%d'/1'/%d",
				PurposeBLS12381, td.vault.CoinType, path.AddressIndex()), info.Path)
		case H(PurposeImportPrivateKey):
			assert.Equal(t, fmt.Sprintf("m/%d'/%d'/1'/%d'",
				PurposeImportPrivateKey, td.vault.CoinType, path.AddressIndex()-hdkeychain.HardenedKeyStart), info.Path)
		default:
			assert.Fail(t, "not supported")
		}
	}
}

func TestSortAllValidatorAddresses(t *testing.T) {
	td := setup(t)

	validatorAddrs := td.vault.AllValidatorAddresses()

	assert.Equal(t, "m/12381'/21888'/1'/0", validatorAddrs[0].Path)
	assert.Equal(t, "m/65535'/21888'/1'/0'", validatorAddrs[len(validatorAddrs)-1].Path)
}

func TestAddressFromPath(t *testing.T) {
	td := setup(t)

	t.Run("Could not find address from path", func(t *testing.T) {
		path := "m/12381'/26888'/983'/0"
		assert.Nil(t, td.vault.AddressFromPath(path))
	})

	t.Run("Ok", func(t *testing.T) {
		var address string
		var addrInfo AddressInfo

		for addr, ai := range td.vault.Addresses {
			address = addr
			addrInfo = ai

			break
		}

		assert.Equal(t, address, td.vault.AddressFromPath(addrInfo.Path).Address)
	})
}

func TestAllImportedPrivateKeysAddresses(t *testing.T) {
	td := setup(t)

	importedPrvAddrs := td.vault.AllImportedPrivateKeysAddresses()
	for _, i := range importedPrvAddrs {
		info := td.vault.AddressInfo(i.Address)
		assert.Equal(t, i.Address, info.Address)

		addr, _ := crypto.AddressFromString(info.Address)
		path, _ := addresspath.FromString(info.Path)

		if addr.IsValidatorAddress() {
			assert.Equal(t, fmt.Sprintf("m/%d'/%d'/1'/%d'",
				PurposeImportPrivateKey, td.vault.CoinType, path.AddressIndex()-hdkeychain.HardenedKeyStart), info.Path)
		}

		if addr.IsAccountAddress() {
			assert.Equal(t, fmt.Sprintf("m/%d'/%d'/2'/%d'",
				PurposeImportPrivateKey, td.vault.CoinType, path.AddressIndex()-hdkeychain.HardenedKeyStart), info.Path)
		}
	}
}

func TestNewValidatorAddress(t *testing.T) {
	td := setup(t)

	label := td.RandString(16)
	addressInfo, err := td.vault.NewValidatorAddress(label)
	assert.NoError(t, err)
	assert.NotEmpty(t, addressInfo.Address)
	assert.NotEmpty(t, addressInfo.PublicKey)
	assert.Contains(t, addressInfo.Path, "m/12381'/21888'/1'")
	assert.Equal(t, label, addressInfo.Label)

	pub, _ := bls.PublicKeyFromString(addressInfo.PublicKey)
	assert.Equal(t, pub.ValidatorAddress().String(), addressInfo.Address)
}

func TestNewBLSAccountAddress(t *testing.T) {
	td := setup(t)

	label := td.RandString(16)
	addressInfo, err := td.vault.NewBLSAccountAddress(label)
	assert.NoError(t, err)
	assert.NotEmpty(t, addressInfo.Address)
	assert.NotEmpty(t, addressInfo.PublicKey)
	assert.Contains(t, addressInfo.Path, "m/12381'/21888'/2'")
	assert.Equal(t, label, addressInfo.Label)

	pub, _ := bls.PublicKeyFromString(addressInfo.PublicKey)
	assert.Equal(t, pub.AccountAddress().String(), addressInfo.Address)
}

func TestNewE225519AccountAddress(t *testing.T) {
	td := setup(t)

	addressInfo, err := td.vault.NewEd25519AccountAddress("addr-2", tPassword)
	assert.NoError(t, err)
	assert.NotEmpty(t, addressInfo.Address)
	assert.NotEmpty(t, addressInfo.PublicKey)
	assert.Equal(t, "m/44'/21888'/3'/1'", addressInfo.Path)

	pub, _ := ed25519.PublicKeyFromString(addressInfo.PublicKey)
	assert.Equal(t, pub.AccountAddress().String(), addressInfo.Address)
}

func TestRecover(t *testing.T) {
	td := setup(t)

	t.Run("Invalid mnemonic", func(t *testing.T) {
		_, err := CreateVaultFromMnemonic("invalid mnemonic phrase seed", 21888)
		assert.Error(t, err)
	})

	t.Run("Ok", func(t *testing.T) {
		recovered, err := CreateVaultFromMnemonic(td.mnemonic, 21888)
		assert.NoError(t, err)

		// Recover addresses
		_, err = recovered.NewBLSAccountAddress("bls-account-address")
		assert.NoError(t, err)
		_, err = recovered.NewEd25519AccountAddress("ed25519-account-address", "")
		assert.NoError(t, err)
		_, err = recovered.NewValidatorAddress("validator-address")
		assert.NoError(t, err)

		assert.Equal(t, recovered.Purposes, td.vault.Purposes)
	})
}

func TestGetPrivateKeys(t *testing.T) {
	td := setup(t)

	t.Run("Unknown address", func(t *testing.T) {
		addr := td.RandAccAddress()
		_, err := td.vault.PrivateKeys(tPassword, []string{addr.String()})
		assert.ErrorIs(t, err, NewErrAddressNotFound(addr.String()))
	})

	t.Run("No password", func(t *testing.T) {
		addr := td.vault.AddressInfos()[0].Address
		_, err := td.vault.PrivateKeys("", []string{addr})
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
	})

	t.Run("Invalid password", func(t *testing.T) {
		addr := td.vault.AddressInfos()[0].Address
		_, err := td.vault.PrivateKeys("wrong_password", []string{addr})
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
	})

	t.Run("Check all the private keys", func(t *testing.T) {
		for _, info := range td.vault.AddressInfos() {
			prv, err := td.vault.PrivateKeys(tPassword, []string{info.Address})
			assert.NoError(t, err)
			addrInfo := td.vault.AddressInfo(info.Address)
			path, _ := addresspath.FromString(info.Path)

			switch path.AddressType() - addresspath.HardenedKeyStart {
			case uint32(crypto.AddressTypeBLSAccount),
				uint32(crypto.AddressTypeValidator):
				pub, _ := bls.PublicKeyFromString(addrInfo.PublicKey)
				require.True(t, prv[0].PublicKey().EqualsTo(pub))
			case uint32(crypto.AddressTypeEd25519Account):
				pub, _ := ed25519.PublicKeyFromString(addrInfo.PublicKey)
				require.True(t, prv[0].PublicKey().EqualsTo(pub))
			default:
				assert.Fail(t, "not supported")
			}
		}
	})
}

func TestImportPrivateKey(t *testing.T) {
	td := setup(t)

	t.Run("Reimporting private key", func(t *testing.T) {
		err := td.vault.ImportBLSPrivateKey(tPassword, td.importedPrv.(*bls.PrivateKey))
		assert.ErrorIs(t, err, ErrAddressExists)
	})

	t.Run("Invalid password", func(t *testing.T) {
		_, prv := td.RandBLSKeyPair()
		err := td.vault.ImportBLSPrivateKey("invalid-password", prv)
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
	})

	t.Run("Ok", func(t *testing.T) {
		_, prv := td.RandBLSKeyPair()
		assert.NoError(t, td.vault.ImportBLSPrivateKey(tPassword, prv))
		assert.True(t, td.vault.Contains(prv.PublicKeyNative().AccountAddress().String()))
		assert.True(t, td.vault.Contains(prv.PublicKeyNative().ValidatorAddress().String()))
	})
}

func TestGetMnemonic(t *testing.T) {
	td := setup(t)

	t.Run("Invalid password", func(t *testing.T) {
		_, err := td.vault.Mnemonic("invalid-password")
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
	})

	t.Run("No password", func(t *testing.T) {
		_, err := td.vault.Mnemonic("")
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
	})

	t.Run("Ok", func(t *testing.T) {
		m, err := td.vault.Mnemonic(tPassword)
		assert.NoError(t, err)
		assert.Equal(t, m, td.mnemonic)
	})

	t.Run("Neutered wallet", func(t *testing.T) {
		_, err := td.vault.Neuter().Mnemonic("")
		assert.ErrorIs(t, err, ErrNeutered)
	})
}

func TestUpdatePassword(t *testing.T) {
	td := setup(t)

	infos := make([]*AddressInfo, 0, td.vault.AddressCount())
	for _, info := range td.vault.AddressInfos() {
		info := td.vault.AddressInfo(info.Address)
		infos = append(infos, info)
	}

	newPassword := "new-password"
	t.Run("Change password", func(t *testing.T) {
		opts := []encrypter.Option{
			encrypter.OptionIteration(1),
			encrypter.OptionMemory(1),
			encrypter.OptionParallelism(1),
		}

		err := td.vault.UpdatePassword("", newPassword)
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
		err = td.vault.UpdatePassword("invalid-password", newPassword)
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
		assert.NoError(t, td.vault.UpdatePassword(tPassword, newPassword, opts...))
		assert.True(t, td.vault.IsEncrypted())
		for _, info := range infos {
			assert.Equal(t, info, td.vault.AddressInfo(info.Address))
		}
	})

	t.Run("Set empty password for the vault", func(t *testing.T) {
		err := td.vault.UpdatePassword("invalid-password", newPassword)
		assert.ErrorIs(t, err, encrypter.ErrInvalidPassword)
		assert.NoError(t, td.vault.UpdatePassword(newPassword, ""))
		assert.False(t, td.vault.IsEncrypted())
		for _, info := range infos {
			assert.Equal(t, info, td.vault.AddressInfo(info.Address))
		}
	})
}

func TestSetLabel(t *testing.T) {
	td := setup(t)

	t.Run("Set label for unknown address", func(t *testing.T) {
		invAddr := td.RandAccAddress().String()
		err := td.vault.SetLabel(invAddr, "i have label")
		assert.ErrorIs(t, err, NewErrAddressNotFound(invAddr))
		assert.Equal(t, "", td.vault.Label(invAddr))
	})

	t.Run("Update label", func(t *testing.T) {
		testAddr := td.vault.AddressInfos()[0].Address
		err := td.vault.SetLabel(testAddr, "I have a label")
		assert.NoError(t, err)
		assert.Equal(t, "I have a label", td.vault.Label(testAddr))
	})

	t.Run("Remove label", func(t *testing.T) {
		testAddr := td.vault.AddressInfos()[0].Address
		err := td.vault.SetLabel(testAddr, "")
		assert.NoError(t, err)
		var ok bool
		l := td.vault.Label(testAddr)
		if strings.TrimSpace(l) != "" {
			ok = true
		}
		assert.Empty(t, td.vault.Label(testAddr))
		assert.False(t, ok)
	})
}

func TestNeuter(t *testing.T) {
	td := setup(t)

	neutered := td.vault.Neuter()
	_, err := neutered.Mnemonic(tPassword)
	assert.ErrorIs(t, err, ErrNeutered)

	_, err = neutered.PrivateKeys(tPassword, []string{
		td.RandAccAddress().String(),
	})
	assert.ErrorIs(t, err, ErrNeutered)

	err = neutered.ImportBLSPrivateKey("any", td.importedPrv.(*bls.PrivateKey))
	assert.ErrorIs(t, err, ErrNeutered)

	err = td.vault.Neuter().UpdatePassword("any", "any")
	assert.ErrorIs(t, err, ErrNeutered)
}
