// this file is obsolete
// signature option should instead be addded to digest.go
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/swarm/storage/mru"
)

var (
	g_dir      string
	g_arg      string
	g_file     string
	g_help     bool
	g_passfile string
	g_pass     string
)

func main() {

	usr, err := user.Current()
	if err != nil {
		log.Error("Could not get user info to resolve homedir")
		os.Exit(1)
	}

	var debug bool
	defaultdatadir := fmt.Sprintf("%s/.ethereum", usr.HomeDir)
	flag.StringVar(&g_dir, "d", defaultdatadir, "datadir")
	flag.BoolVar(&g_help, "h", false, "show help")
	flag.BoolVar(&debug, "v", false, "show debug info")
	flag.StringVar(&g_passfile, "p", "", "passwordfile")

	flag.Usage = func() {
		fmt.Println(`
******************************
* WARNING! WARNING! WARNING! *
******************************

this program will output your private key in hex format to the terminal. Your private key should be kept secret, at the risk of losing all funds within it, and possibly any accounts derived from it.

Some recommended precautions:
* Make sure noone is looking over your shoulder when you run this program.
* Never store your private key on a digital device with insufficient encryption
* Only use this tool on an airgapped machine

The author of this application assumes no warranty or liability.

----
Usage: ethexport [flags] <account hex|keyfile>")

If argument is account hex, the keystore subdir of the datadir will be searched for a matching account
If argument is keyfile, the -d flag will be ignored
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if g_help {
		flag.Usage()
		os.Exit(0)
	}

	lvl := log.LvlError
	if debug {
		lvl = log.LvlDebug
	}
	h := log.LvlFilterHandler(lvl, log.StderrHandler)
	log.Root().SetHandler(h)

	if g_dir == "" {
		g_dir = defaultdatadir
	}

	if g_passfile != "" {
		pass, err := ioutil.ReadFile(g_passfile)
		if err != nil {
			log.Error("password file %s doesn't exist", g_passfile)
			os.Exit(1)
		}
		g_pass = string(pass)
	}

	if flag.Arg(0) == "" {
		log.Error("Account or keyfile must be specified")
		os.Exit(1)
	}
	g_arg = flag.Arg(0)

	if flag.Arg(1) == "" {
		log.Error("json request file must be specified")
		os.Exit(1)
	}
	g_file = flag.Arg(1)

	// check if we have file or account
	var keyfile string
	if _, err := hexutil.Decode(g_arg); err != nil {
		log.Debug("input is keyfile")
		fi, err := os.Stat(g_arg)
		if err != nil {
			log.Error("Keyfile not found", "path", g_arg)
			os.Exit(1)
		} else if fi.IsDir() {
			log.Error("Keyfile argument is a directory", "path", g_arg)
			os.Exit(1)
		}
		keyfile = g_arg
	} else {
		log.Debug("input is account hex")
		fi, err := os.Stat(g_dir)
		if err != nil {
			log.Error("Keystore not found", "path", g_dir)
			os.Exit(1)
		} else if !fi.IsDir() {
			log.Error("Keystore is not a directory", "path", g_dir)
			os.Exit(1)
		}

		// search the directory for the key
		keystoredir := fmt.Sprintf("%s/keystore", g_dir)
		log.Debug("checking keystore dir", "dir", keystoredir)
		dircontents, err := ioutil.ReadDir(keystoredir)
		if err != nil {
			log.Error("Can't open keystore dir: %v", err)
		}
		for _, f := range dircontents {
			if strings.Contains(f.Name(), g_arg[2:]) {
				keyfile = fmt.Sprintf("%s/%s", keystoredir, f.Name())
			}
		}
	}

	if keyfile == "" {
		log.Error("Account not found")
		os.Exit(1)
	}

	log.Info("opening account", "keyfile", keyfile)
	j, err := ioutil.ReadFile(keyfile)
	if err != nil {
		log.Error("cannot read file", "err", err)
		os.Exit(1)
	}
	if g_pass == "" {
		fmt.Printf("pass:")
		bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Error("read password fail", "err", err)
			os.Exit(1)
		}
		g_pass = string(bytePassword)
	}
	log.Info("decrypting keyfile... with %s", g_pass)
	key, err := keystore.DecryptKey(j, g_pass)
	if err != nil {
		log.Error("key decrypt failed", "err", err)
		os.Exit(1)
	}

	privkeyhex := hex.EncodeToString(crypto.FromECDSA(key.PrivateKey))
	log.Debug("priv", "hex", privkeyhex)
	privkeyregen, err := crypto.HexToECDSA(privkeyhex)
	if err != nil {
		log.Error("internal privkey conversion error", "err", err)
		os.Exit(1)
	}
	log.Info("ok", "privkey", privkeyhex, "address", crypto.PubkeyToAddress(privkeyregen.PublicKey))
	//fmt.Println(privkeyhex)

	requestJson, err := ioutil.ReadFile(g_file)
	if err != nil {
		log.Error("resultjson fail: ", "err", err.Error())
		os.Exit(1)
	}
	request := &mru.Request{}
	err = request.UnmarshalJSON(requestJson)
	if err != nil {
		log.Error("unmarshaljson fail", "err", err.Error())
		os.Exit(1)
	}
	resultJson, err := request.MarshalJSON()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	log.Info("parsed requestjson", "json", string(resultJson))
	digest, err := request.GetDigest()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	log.Info("digest", "x", hexutil.Encode(digest.Bytes()))

	signer := mru.NewGenericSigner(key.PrivateKey)
	request.Sign(signer)

	resultJson, err = request.MarshalJSON()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	fmt.Println(string(resultJson))
}
