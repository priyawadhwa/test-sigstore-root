package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkeys "github.com/asraa/test-sigstore-root/pkg/keys"
	prepo "github.com/asraa/test-sigstore-root/pkg/repo"
	"github.com/theupdateframework/go-tuf/data"

	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/theupdateframework/go-tuf"
)

type keysFlag []string

func (f *keysFlag) String() string {
	return strings.Join(*f, ", ")
}

func (f *keysFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func AddDelegation() *ffcli.Command {
	var (
		flagset    = flag.NewFlagSet("tuf add-delegation", flag.ExitOnError)
		repository = flagset.String("repository", "", "path to the staged repository")
		name       = flagset.String("name", "", "name of the delegatee")
		keys       = keysFlag{}
		path       = flagset.String("path", "", "path for the delegation")
		targets    = flagset.String("target-meta", "", "path to a target configuration file")
	)
	flagset.Var(&keys, "key", "key reference for the delegatee")
	return &ffcli.Command{
		Name:       "add-delegation",
		ShortUsage: "tuf add-delegation a role delegation from the top-level targets",
		ShortHelp:  "tuf add-delegation a role delegation from the top-level targets",
		LongHelp: `tuf add-delegation a role delegation from the top-level targets.
		Adds a targets delegation with a name and specified keys. The optional path can also be set, 
but will default to the name if unspecified.
		
	EXAMPLES
	# add-delegation repository at ceremony/YYYY-MM-DD
	tuf add-delegation -repository ceremony/YYYY-MM-DD -name $NAME -key $KEY_A -key $KEY_B -path $PATH`,
		FlagSet: flagset,
		Exec: func(ctx context.Context, args []string) error {
			if *repository == "" {
				return flag.ErrHelp
			}
			if *name == "" {
				return flag.ErrHelp
			}
			if len(keys) == 0 {
				return flag.ErrHelp
			}
			return DelegationCmd(ctx, *repository, *name, *path, keys, *targets)
		},
	}
}

func DelegationCmd(ctx context.Context, directory, name, path string, keyRefs keysFlag, targets string) error {
	store := tuf.FileSystemStore(directory, nil)

	repo, err := tuf.NewRepoIndent(store, "", "\t", "sha512", "sha256")
	if err != nil {
		return err
	}
	if path == "" {
		path = name
	}

	// Store signature placeholders
	s, err := prepo.GetSignedMeta(store, "targets.json")
	if err != nil {
		return err
	}
	sigs := s.Signatures

	keys := []*data.PublicKey{}
	ids := []string{}
	for _, keyRef := range keyRefs {
		signerKey, err := pkeys.GetKmsSigningKey(ctx, keyRef)
		if err != nil {
			return err
		}
		keys = append(keys, signerKey.Key)
		ids = append(ids, signerKey.Key.IDs()...)
	}
	// Don't increment targets version multiple times.
	version, err := repo.TargetsVersion()
	if err != nil {
		return err
	}

	expiration := time.Now().AddDate(0, 6, 0).UTC()
	if err := repo.AddTargetsDelegationWithExpires("targets", data.DelegatedRole{
		Name:      name,
		KeyIDs:    ids,
		Paths:     []string{path},
		Threshold: 1,
	}, keys, expiration); err != nil {
		// If delegation already added, then we just want to bump version and expiration.
		fmt.Fprintln(os.Stdout, "Adding targets delegation: ", err)
	}

	// Add targets (copy them into the repository and add them to the targets.json)
	if targets != "" {
		targetCfg, err := os.ReadFile(targets)
		if err != nil {
			return err
		}
		meta, err := prepo.TargetMetaFromString(targetCfg)
		if err != nil {
			return err
		}

		for tt, custom := range meta {
			from, err := os.Open(tt)
			if err != nil {
				return err
			}
			defer from.Close()
			base := filepath.Base(tt)
			to, err := os.Create(filepath.Join(directory, "staged", "targets", base))
			if err != nil {
				return err
			}
			defer to.Close()
			if _, err := io.Copy(to, from); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Created target file at ", to.Name())
			if err := repo.AddTargetsWithExpiresToPreferredRole([]string{base}, custom, expiration, name); err != nil {
				return fmt.Errorf("error adding targets %w", err)
			}
		}
	}

	if err := repo.SetTargetsVersion(version); err != nil {
		return err
	}

	// Recover the blank signatures on targets
	t, err := prepo.GetTargetsFromStore(store)
	if err != nil {
		return err
	}
	signed, err := jsonMarshal(t)
	if err != nil {
		return err
	}
	return setSignedMeta(store, "targets.json", &data.Signed{Signatures: sigs, Signed: signed})
}
