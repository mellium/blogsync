// Copyright 2019 The Blog Sync Contributors.
// Use of this source code is governed by the BSD 2-clause
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"text/template"
	"time"

	"github.com/writeas/go-writeas/v2"
	"mellium.im/blogsync/internal/browser"
	"mellium.im/cli"
)

const (
	dbFileName  = "data.db"
	cfgFileName = "config.ini"
	adminUser   = "root"
	/* #nosec */
	adminPass = "password"
	binName   = "writefreely"
)

type writeFreelyConfig struct {
	Bind            string
	Collection      string
	DBFile          string
	Port            int
	Prefix          string
	Resources       string
	SiteDescription string
	SiteName        string
}

const tmpFileTmpl = `
[server]
hidden_host          =
port                 = {{.Port}}
bind                 = {{.Bind}}
tls_cert_path        =
tls_key_path         =
autocert             = false
templates_parent_dir = {{.Resources}}
static_parent_dir    = {{.Resources}}
pages_parent_dir     = {{.Resources}}
keys_parent_dir      = {{.Prefix}}

[database]
type     = sqlite3
filename = {{.Prefix}}/{{.DBFile}}
username =
password =
database =
host     = localhost
port     = 3306

[app]
site_name          = {{.SiteName}}
site_description   = {{.SiteDescription}}
host               = http://{{.Bind}}:{{.Port}}
theme              = write
editor             =
disable_js         = false
webfonts           = true
landing            = /{{.Collection}}
simple_nav         = false
wf_modesty         = false
chorus             = false
disable_drafts     = false
single_user        = false
open_registration  = false
min_username_len   = 3
max_blogs          = 2
federation         = false
public_stats       = false
private            = false
local_timeline     = false
user_invites       =
default_visibility = public
`

func previewCmd(siteConfig Config, logger, debug *log.Logger) *cli.Command {
	var (
		port    = 8080
		bind    = "127.0.0.1"
		content = "content/"
		res     = "/usr/share/writefreely/"
	)
	flags := flag.NewFlagSet("preview", flag.ContinueOnError)
	flags.IntVar(&port, "port", port, "The port for writefreely to bind to")
	flags.StringVar(&bind, "addr", bind, "The address the server should bind to")
	flags.StringVar(&content, "content", content, "A directory containing pages and posts")
	flags.StringVar(&res, "resources", res, "A directory containing writefreelys templates and static assets")

	return &cli.Command{
		Usage:       "preview [options]",
		Flags:       flags,
		Description: `Launch writefreely and upload current pages.`,
		Run: func(cmd *cli.Command, args ...string) error {
			// Override the default SIGINT handler so that we can cleanup properly on
			// Ctrl+C instead of immediately exiting.
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, os.Interrupt)

			_, err := exec.LookPath(binName)
			if err != nil {
				return fmt.Errorf(`
The 'writefreely' command could not be found.
To use the preview feature, please install writefreely:

https://writefreely.org/

(original error: %w)`, err)
			}

			tmpDir, err := mkTmp(writeFreelyConfig{
				Bind:            bind,
				Collection:      siteConfig.Collection,
				DBFile:          dbFileName,
				Port:            port,
				Resources:       res,
				SiteDescription: siteConfig.Description,
				SiteName:        siteConfig.Title,
			}, debug)
			if err != nil {
				return fmt.Errorf("can't create temporary directories: %v", err)
			}
			defer func() {
				err := os.RemoveAll(tmpDir)
				if err != nil {
					debug.Printf("error removing temporary dir %s: %v", tmpDir, err)
				}
			}()

			var cfgFilePath = filepath.Join(tmpDir, cfgFileName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err = tailWriteFreely(ctx, cfgFilePath, debug, "-gen-keys")
			if err != nil {
				return err
			}
			err = tailWriteFreely(ctx, cfgFilePath, debug, "-init-db")
			if err != nil {
				return err
			}
			err = tailWriteFreely(ctx, cfgFilePath, debug, "-create-admin", fmt.Sprintf("%s:%s", adminUser, adminPass))
			if err != nil {
				return err
			}

			go func() {
				err = tailWriteFreely(ctx, cfgFilePath, debug)
				if err != nil {
					debug.Printf("error while executing writefreely: %v", err)
				}
				cancel()
			}()

			addr := net.JoinHostPort(bind, strconv.Itoa(port))

			// Wait until writefreely becomes available.
			var connected bool
			for i := 0; i < 5; i++ {
				const timeout = 1 * time.Second
				logger.Printf("waiting %s for writefreely to accept connections…", timeout)
				conn, err := net.Dial("tcp", addr)
				if err == nil {
					err = conn.Close()
					if err != nil {
						debug.Printf("error closing temporary TCP connection: %v", err)
					}
					logger.Println("connected to writefreely!")
					connected = true
					break
				}
				time.Sleep(timeout)
			}
			if !connected {
				return fmt.Errorf("failed to connect to writefreely, did it start?")
			}

			baseAddr := "http://" + addr
			client := writeas.NewClientWith(writeas.Config{
				URL: baseAddr + "/api",
			})
			authUser, err := client.LogIn(adminUser, adminPass)
			if err != nil {
				return err
			}
			debug.Printf("logged in as: %+v", authUser)

			opts := newPublishOpts(siteConfig)
			opts.createCollections = true

			err = publish(opts, siteConfig, client, logger, debug)
			if err != nil {
				return err
			}

			browser.Open(baseAddr)

			select {
			case <-sigs:
			case <-ctx.Done():
			}
			return nil
		},
	}
}

func tailWriteFreely(ctx context.Context, cfgFile string, debug *log.Logger, args ...string) error {
	args = append([]string{"-c", cfgFile}, args...)
	cmd := exec.CommandContext(ctx, binName, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Dir = filepath.Dir(cfgFile)

	debug.Printf("running %s with %v…\n", cmd.Path, cmd.Args)

	return cmd.Run()
}

func writeConfig(cfgFileName string, cfg writeFreelyConfig, debug *log.Logger) (err error) {
	cfgFile, err := os.Create(cfgFileName)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err := os.Remove(cfgFile.Name())
			if err != nil {
				debug.Printf("error during early removal of temporary config file %s: %v", cfgFile.Name(), err)
			}
		}
	}()

	t := template.Must(template.New("cfg").Parse(tmpFileTmpl))
	err = t.Execute(cfgFile, cfg)
	if err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	err = cfgFile.Close()
	if err != nil {
		return fmt.Errorf("error closing temporary config file %s: %w", cfgFile.Name(), err)
	}

	return nil
}

func mkTmp(cfg writeFreelyConfig, debug *log.Logger) (tmpDir string, e error) {
	const (
		mode = os.ModeDir | 0755
	)

	tmpDir, err := ioutil.TempDir("", "blogsync")
	if err != nil {
		return tmpDir, err
	}
	defer func() {
		if e != nil {
			err := os.RemoveAll(tmpDir)
			if err != nil {
				debug.Printf("error during early removal of temporary dir %s: %v", tmpDir, err)
			}
		}
	}()
	cfg.Prefix = tmpDir

	for _, dir := range []string{"keys", "pages", "static", "templates"} {
		err := os.Mkdir(filepath.Join(tmpDir, dir), mode)
		if err != nil {
			return tmpDir, err
		}
	}

	dbFile, err := os.Create(filepath.Join(tmpDir, cfg.DBFile))
	if err != nil {
		return tmpDir, err
	}
	err = dbFile.Close()
	if err != nil {
		return tmpDir, err
	}

	err = writeConfig(filepath.Join(tmpDir, cfgFileName), cfg, debug)
	if err != nil {
		return tmpDir, err
	}

	return tmpDir, nil
}
