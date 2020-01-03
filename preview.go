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
	"syscall"
	"text/template"
	"time"

	"github.com/writeas/go-writeas/v2"
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

			err = runWriteFreely(cfgFilePath, debug, "-gen-keys")
			if err != nil {
				return err
			}
			err = runWriteFreely(cfgFilePath, debug, "-init-db")
			if err != nil {
				return err
			}
			err = runWriteFreely(cfgFilePath, debug, "-create-admin", fmt.Sprintf("%s:%s", adminUser, adminPass))
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err = tailWriteFreely(ctx, sigs, cfgFilePath, debug)
			if err != nil {
				debug.Printf("error while executing writefreely: %v", err)
			}

			// TODO: this is jank. Manually spin up the process and wait on a log line
			// or something (which is only slightly less jank, yay shelling out).
			time.Sleep(3 * time.Second)

			client := writeas.NewClientWith(writeas.Config{
				URL: "http://" + net.JoinHostPort(bind, strconv.Itoa(port)+"/api"),
			})
			authUser, err := client.LogIn(adminUser, adminPass)
			if err != nil {
				return err
			}
			debug.Printf("logged in as: %+v", authUser)

			// TODO: add a mechanism for forcing the creation of collections that
			// don't exist during publishing, and then rely on that.
			_, err = client.CreateCollection(&writeas.CollectionParams{
				Alias:       siteConfig.Collection,
				Title:       siteConfig.Title,
				Description: siteConfig.Description,
			})
			if err != nil {
				return err
			}

			err = publishCmd(siteConfig, client, logger, debug).Exec()
			if err != nil {
				return err
			}

			<-sigs
			return nil
		},
	}
}

func runWriteFreely(cfgFile string, debug *log.Logger, args ...string) error {
	args = append([]string{"-c", cfgFile}, args...)
	cmd := exec.Command(binName, args...)
	debug.Printf("running %s with %v…\n--Start output--", cmd.Path, cmd.Args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	fmt.Fprintf(debug.Writer(), "%s\n--End of output--\n", output)
	return nil
}

func tailWriteFreely(ctx context.Context, sigs chan<- os.Signal, cfgFile string, debug *log.Logger) error {
	cmd := exec.CommandContext(ctx, binName, "-c", cfgFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Dir = filepath.Dir(cfgFile)

	debug.Printf("running %s with %v…\n", cmd.Path, cmd.Args)

	// Also exit if SIGCHLD is sent (writefreely died).
	signal.Notify(sigs, syscall.SIGCHLD)
	return cmd.Start()
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
