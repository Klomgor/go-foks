// Copyright (c) 2025 ne43, Inc.
// Licensed under the MIT License. See LICENSE in the project root for details.

package cmd

import (
	"bytes"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"github.com/foks-proj/go-foks/client/agent"
	"github.com/foks-proj/go-foks/client/libclient"
	"github.com/foks-proj/go-foks/client/libkv"
	"github.com/foks-proj/go-foks/lib/core"
	"github.com/foks-proj/go-foks/lib/libterm"
	"github.com/foks-proj/go-foks/proto/lcl"
	proto "github.com/foks-proj/go-foks/proto/lib"
	"github.com/spf13/cobra"
)

var kvOpts = agent.StartupOpts{
	NeedUser:         true,
	NeedUnlockedUser: true,
}

type quickKVOpts struct {
	SupportReadRole   bool
	SupportWriteRole  bool
	NoSupportMkdirP   bool
	SupportOverwrite  bool
	SupportMtimeLower bool
	SupportRecursive  bool
}

func (q quickKVOpts) SupportsRoles() bool {
	return q.SupportReadRole || q.SupportWriteRole
}

func actAsTeamOpt(
	cmd *cobra.Command,
	teamStr *string,
) {
	cmd.Flags().StringVarP(teamStr, "team", "t", "", "team to work on behalf of (default is to operate as the logged in user)")
}

func makeKVPath(s string) proto.KVPath {
	// noop unless we are in a gitbash environment on windows
	return libclient.GitBashAbsPathInvert(proto.KVPath(s))
}

func quickKVCmd(
	m libclient.MetaContext,
	top *cobra.Command,
	name string,
	aliases []string,
	short string,
	long string,
	opts quickKVOpts,
	setup func(*cobra.Command),
	fn func([]string, lcl.KVConfig, lcl.KVClient) error,
) {
	if long == "" {
		long = short
	}
	var teamStr string
	var rrs, wrs string
	var rr, wr *proto.Role
	var mtimeStr string
	var mkdirP bool
	var force bool
	var recursive bool
	var mtime *proto.TimeMicro
	run := func(cmd *cobra.Command, arg []string) error {
		var fqt *proto.FQTeamParsed
		if teamStr != "" {
			var err error
			fqt, err = core.ParseFQTeam(proto.FQTeamString(teamStr))
			if err != nil {
				return err
			}
		}
		if opts.SupportReadRole && rrs != "" {
			var err error
			rs := proto.RoleString(rrs)
			rr, err = rs.Parse()
			if err != nil {
				return err
			}
		}
		if opts.SupportWriteRole && wrs != "" {
			var err error
			rs := proto.RoleString(wrs)
			wr, err = rs.Parse()
			if err != nil {
				return err
			}
		}
		if opts.SupportMtimeLower && mtimeStr != "" {
			t, err := time.Parse(time.RFC3339, mtimeStr)
			if err != nil {
				return err
			}
			tmp := proto.ExportTimeMicro(t)
			mtime = &tmp
		}
		cfg := lcl.KVConfig{
			ActingAs:    fqt,
			Roles:       proto.RolePairOpt{Read: rr, Write: wr},
			MkdirP:      mkdirP,
			OverwriteOk: force,
			MtimeLower:  mtime,
			Recursive:   recursive,
		}
		return quickStartLambda(m, &kvOpts, func(cli lcl.KVClient) error {
			err := fn(arg, cfg, cli)
			if err != nil {
				return err
			}
			return PartingConsoleMessage(m)
		})
	}

	cmd := &cobra.Command{
		Use:          name,
		Aliases:      aliases,
		Short:        short,
		Long:         long,
		SilenceUsage: true,
		RunE:         run,
	}
	actAsTeamOpt(cmd, &teamStr)
	if !opts.NoSupportMkdirP {
		cmd.Flags().BoolVarP(&mkdirP, "mkdir-p", "p", false, "create parent directories if they do not exist")
	}
	if opts.SupportReadRole {
		cmd.Flags().StringVarP(&rrs, "read-role", "r", "", "read role to create as (default depends on subcommand)")
	}
	if opts.SupportWriteRole {
		cmd.Flags().StringVarP(&wrs, "write-role", "w", "", "write role to create as (default depends on subcommand)")
	}
	if opts.SupportOverwrite {
		cmd.Flags().BoolVar(&force, "force", false, "overwrite existing key-value store entry")
	}
	if opts.SupportMtimeLower {
		cmd.Flags().StringVar(&mtimeStr, "mtime-lower", "", "lower bound for modification time (RFC3339)")
	}
	if opts.SupportRecursive {
		cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "operate recursively")
	}
	if setup != nil {
		setup(cmd)
	}
	top.AddCommand(cmd)
}

func kvCmd(m libclient.MetaContext) *cobra.Command {
	top := &cobra.Command{
		Use:          "kv",
		Short:        "key-value store commands",
		Long:         "key-value store put/get and management commands",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return subcommandHelp(cmd, args)
		},
	}
	kvMkdir(m, top)
	kvPut(m, top)
	kvGet(m, top)
	kvSymlink(m, top)
	kvMv(m, top)
	kvLs(m, top)
	kvRm(m, top)
	kvReadlink(m, top)
	kvGetUsage(m, top)
	return top
}

func kvReadlink(m libclient.MetaContext, top *cobra.Command) {
	quickKVCmd(m, top,
		"readlink", nil,
		"read a key-value store symlink",
		"Read a key-value store symlink",
		quickKVOpts{},
		nil,
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) != 1 {
				return ArgsError("expected exactly one argument -- the key-value store symlink")
			}
			path := makeKVPath(arg[0])
			res, err := cli.ClientKVReadlink(m.Ctx(), lcl.ClientKVReadlinkArg{
				Cfg:  cfg,
				Path: path,
			})
			if err != nil {
				return err
			}
			if m.G().Cfg().JSONOutput() {
				return JSONOutput(m, res)
			}
			m.G().UIs().Terminal.Printf("%s\n", res)
			return PartingConsoleMessage(m)
		},
	)
}

func kvMv(m libclient.MetaContext, top *cobra.Command) {
	quickKVCmd(m, top,
		"mv", []string{"move", "rename"},
		"move a key-value store entry",
		"Move a key-value store entry",
		quickKVOpts{SupportWriteRole: true, SupportReadRole: true},
		nil,
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) != 2 {
				return ArgsError("expected exactly 2 arguments -- the source and the destination")
			}
			src := makeKVPath(arg[0])
			dst := makeKVPath(arg[1])
			err := cli.ClientKVMv(m.Ctx(), lcl.ClientKVMvArg{
				Cfg: cfg,
				Src: src,
				Dst: dst,
			})
			if err != nil {
				return err
			}
			return PartingConsoleMessage(m)
		},
	)
}

func kvGetUsage(m libclient.MetaContext, top *cobra.Command) {
	quickKVCmd(m, top,
		"get-usage", []string{"du"},
		"get key-value store usage",
		"Get key-value store usage",
		quickKVOpts{},
		nil,
		func(args []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(args) != 0 {
				return ArgsError("expected no arguments")
			}
			res, err := cli.ClientKVUsage(m.Ctx(), cfg)
			if err != nil {
				return err
			}
			if m.G().Cfg().JSONOutput() {
				return JSONOutput(m, res)
			}
			m.G().UIs().Terminal.Printf(
				"Num Files: %d\n"+
					"Total Size: %d\n",
				res.Small.Num+res.Large.Base.Num,
				res.Small.Sum+res.Large.Base.Sum,
			)
			return PartingConsoleMessage(m)
		},
	)
}

func kvRm(m libclient.MetaContext, top *cobra.Command) {
	quickKVCmd(m, top,
		"rm <key1> <key2> ....",
		[]string{"remove", "unlink", "delete"},
		"remove a key-value store entry",
		"Remove a key-value store entry; supply -r to remove directories",
		quickKVOpts{
			SupportReadRole:  true,
			SupportWriteRole: true,
			SupportRecursive: true,
		},
		nil,
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) < 1 {
				return ArgsError("expected at least one argument -- the key-value store entry to remove")
			}
			for _, a := range arg {
				err := cli.ClientKVRm(
					m.Ctx(),
					lcl.ClientKVRmArg{
						Cfg:  cfg,
						Path: makeKVPath(a),
					},
				)
				if err != nil {
					return err
				}
			}
			return PartingConsoleMessage(m)
		},
	)
}

func kvSymlink(m libclient.MetaContext, top *cobra.Command) {
	quickKVCmd(m, top,
		"symlink <key> <target>", []string{"ln"},
		"create a key-value store symlink",
		"Create a key-value store symlink",
		quickKVOpts{SupportWriteRole: true, SupportReadRole: true},
		nil,
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) != 2 {
				return ArgsError("expected exactly 2 arguments -- the key and the target")
			}
			path := makeKVPath(arg[0])
			target := makeKVPath(arg[1])
			res, err := cli.ClientKVSymlink(m.Ctx(), lcl.ClientKVSymlinkArg{
				Cfg:    cfg,
				Path:   path,
				Target: target,
			})
			if err != nil {
				return err
			}
			if m.G().Cfg().JSONOutput() {
				return JSONOutput(m, res)
			}
			resStr, err := res.StringErr()
			if err != nil {
				return err
			}
			m.G().UIs().Terminal.Printf("NodeID: %s\n", resStr)
			return PartingConsoleMessage(m)
		},
	)
}

func kvGet(m libclient.MetaContext, top *cobra.Command) {
	var mode int
	var force bool
	var forceOutput bool
	quickKVCmd(m, top,
		"get <key> [<output-file>]", nil,
		"get a key-value store entry",
		libterm.MustRewrapSense(
			`Get a key-value store entry. Supply a key and an optional output file. If
no output file is given, or if the output file is '-', then the value is printed to standard output.
If standard output is a terminal, and the file is probably binary, an error is returned.
This behavior can be overridden by specifying the --force-output flag. `, 0),
		quickKVOpts{},
		func(cmd *cobra.Command) {
			cmd.Flags().IntVarP(&mode, "mode", "", -1, "file mode to use when writing to a file")
			cmd.Flags().BoolVarP(&force, "force", "", false, "overwrite existing file")
			cmd.Flags().BoolVarP(&forceOutput, "force-output", "", false, "force output to terminal even if it looks like binary data")
		},
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) != 2 && len(arg) != 1 {
				return ArgsError("expected 1 or 2 arguments -- the key and the file to write to (or '-' for stdout)")
			}
			out := "-"
			if len(arg) == 2 {
				out = arg[1]
			}
			if mode != -1 && (mode < 0 || mode > 0o777) {
				return ArgsError("mode must be between 0 and 0o777")
			}
			if out == "-" && mode >= 0 {
				return ArgsError("cannot specify file mode when writing to stdout")
			}
			path := makeKVPath(arg[0])
			err := kvGetWithArgs(m, cfg, cli, path, out, mode, force, forceOutput)
			if err != nil {
				return err
			}
			return PartingConsoleMessage(m)
		},
	)
}

func kvPut(m libclient.MetaContext, top *cobra.Command) {
	var isFile bool
	quickKVCmd(m, top,
		"put <key> [<value>]", nil,
		"put a key-value store entry",
		libterm.MustRewrapSense(
			`Put a key-value pair to the store. Supply a key and an option value.
If no value is given, one is read from standard input. If a value is given, it is
interpreted as a string to insert into the store, unless the --file flag is specified.
In that case, the value is interepreted as a file, whose content is read and then
stored under the given key.`,
			0,
		),
		quickKVOpts{SupportWriteRole: true, SupportReadRole: true, SupportOverwrite: true},
		func(cmd *cobra.Command) {
			cmd.Flags().BoolVarP(&isFile, "file", "f", false, "read value from file (or - if from stdin)")
		},
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) != 2 && len(arg) != 1 {
				return ArgsError("expected exactly 1 or 2 arguments -- the key and an optional value")
			}
			var val string
			if len(arg) == 1 {
				isFile = true
				val = "-"
			} else {
				val = arg[1]
			}
			path := makeKVPath(arg[0])
			err := kvPutWithArgs(m, cfg, cli, path, val, isFile)

			// Transform the error to a write error for better online eduction / documentation
			// for remediation (since it might seem weird that a write fails with a read error)
			err = core.ErrorAsWriteError(err)

			if err != nil {
				return err
			}
			return PartingConsoleMessage(m)
		},
	)
}

func openReader(m libclient.MetaContext, value string, isFile bool) (io.Reader, error) {
	if !isFile {
		buf := bytes.NewBufferString(value)
		return buf, nil
	}
	if value == "-" {
		return os.Stdin, nil
	}
	f, err := os.Open(value)
	if err != nil {
		return nil, err
	}
	return f, nil
}

type terminalOutputWrapper struct {
	io.WriteCloser
	didFirst bool
}

func isProbablyBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return true
	}
	for _, b := range data {
		if b < 0x09 || (b > 0x0D && b < 0x20) {
			return true
		}
	}
	return false
}

type TerminalError string

func (e TerminalError) Error() string {
	return string(e)
}

func (t *terminalOutputWrapper) Write(p []byte) (n int, err error) {
	if !t.didFirst {
		t.didFirst = true
		if isProbablyBinary(p) {
			return 0, TerminalError(
				"refusing to output binary data to terminal; use --force-output to override",
			)
		}
	}
	return t.WriteCloser.Write(p)
}

var _ io.WriteCloser = (*terminalOutputWrapper)(nil)

func (t *terminalOutputWrapper) Close() error {
	return t.WriteCloser.Close()
}

func openWriter(
	m libclient.MetaContext,
	dest string,
	mode int,
	force bool,
	forceOutput bool,
) (io.WriteCloser, error) {

	if dest == "-" {
		tui := m.G().UIs().Terminal
		stdout := tui.OutputStream()
		if tui.IsOutputTTY() && !forceOutput {
			return &terminalOutputWrapper{
				WriteCloser: stdout,
				didFirst:    false,
			}, nil
		}
		return stdout, nil
	}

	if mode < 0 {
		mode = 0o600
	}
	flags := os.O_CREATE | os.O_WRONLY
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	return os.OpenFile(dest, flags, os.FileMode(mode))
}

func kvGetWithArgs(
	m libclient.MetaContext,
	cfg lcl.KVConfig,
	cli lcl.KVClient,
	path proto.KVPath,
	dest string,
	mode int,
	force bool,
	forceOutput bool,
) error {
	wrt, err := openWriter(m, dest, mode, force, forceOutput)
	if err != nil {
		return err
	}
	defer wrt.Close()

	return libkv.GetFile(
		wrt,
		func() (lcl.GetFileRes, error) {
			return cli.ClientKVGetFile(m.Ctx(), lcl.ClientKVGetFileArg{
				Cfg:  cfg,
				Path: path,
			})
		},
		func(id proto.FileID, offset proto.Offset) (lcl.GetFileChunkRes, error) {
			return cli.ClientKVGetFileChunk(m.Ctx(), lcl.ClientKVGetFileChunkArg{
				Id:     id,
				Cfg:    cfg,
				Offset: offset,
			})

		},
	)
}

func kvLs(
	m libclient.MetaContext,
	top *cobra.Command,
) {
	quickKVCmd(m, top,
		"ls <key>", []string{"list"},
		"list a key-value store directory",
		"List a key-value store directory, will come back in random order",
		quickKVOpts{
			SupportMtimeLower: true,
		},
		nil,
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) != 1 {
				return ArgsError("expected exactly one argument -- the directory to list")
			}
			path := makeKVPath(arg[0])
			num := m.G().Cfg().KVListPageSize()
			keepGoing := true
			var json []lcl.KVListEntry
			var prefix proto.KVPath
			var dirID *proto.DirID
			nxt := proto.NewKVListPaginationWithNone()
			if cfg.MtimeLower != nil {
				nxt = proto.NewKVListPaginationWithTime(*cfg.MtimeLower)
			}
			for keepGoing {
				res, err := cli.ClientKVList(m.Ctx(), lcl.ClientKVListArg{
					Cfg:   cfg,
					Path:  path,
					Num:   num,
					Nxt:   nxt,
					DirID: dirID,
				})
				if err != nil {
					return err
				}
				if len(prefix) == 0 {
					prefix = res.Parent
				}
				if m.G().Cfg().JSONOutput() {
					json = append(json, res.Ents...)
				} else {
					for _, ent := range res.Ents {
						m.G().UIs().Terminal.Printf("%s%s\n", prefix, ent.Name)
					}
				}
				if res.Nxt != nil {
					dirID = &res.Nxt.Id
					nxt = res.Nxt.Nxt
				} else {
					keepGoing = false
				}
			}

			if len(json) != 0 {
				ret := lcl.CliKVListRes{
					Ents:   json,
					Parent: prefix,
				}
				err := JSONOutput(m, ret)
				if err != nil {
					return err
				}
				return nil
			}

			return PartingConsoleMessage(m)
		},
	)
}

func kvPutWithArgs(
	m libclient.MetaContext,
	cfg lcl.KVConfig,
	cli lcl.KVClient,
	path proto.KVPath,
	value string,
	isFile bool,
) error {
	rdr, err := openReader(m, value, isFile)
	if err != nil {
		return err
	}
	ctx := m.Ctx()
	return libkv.PutFile(
		rdr,
		func(data []byte, isFinal bool) (proto.KVNodeID, error) {
			arg := lcl.ClientKVPutFirstArg{
				Cfg:   cfg,
				Path:  path,
				Chunk: data,
				Final: isFinal,
			}
			return cli.ClientKVPutFirst(ctx, arg)
		},
		func(id proto.FileID, data []byte, offset proto.Offset, final bool) error {
			arg := lcl.ClientKVPutChunkArg{
				Cfg:    cfg,
				Id:     id,
				Chunk:  data,
				Offset: offset,
				Final:  final,
			}
			return cli.ClientKVPutChunk(m.Ctx(), arg)
		},
		0,
	)
}

func kvMkdir(m libclient.MetaContext, top *cobra.Command) {
	quickKVCmd(m, top,
		"mkdir <key>", nil,
		"make a new key-value store directory",
		"Make a new key-value store directory (and parents with -p)",
		quickKVOpts{SupportReadRole: true, SupportWriteRole: true},
		nil,
		func(arg []string, cfg lcl.KVConfig, cli lcl.KVClient) error {
			if len(arg) != 1 {
				return ArgsError("expected exactly one argument -- the key-value store directory name")
			}
			path := makeKVPath(arg[0])
			res, err := cli.ClientKVMkdir(m.Ctx(), lcl.ClientKVMkdirArg{
				Cfg:  cfg,
				Path: path,
			})

			// See comment in kvPut for why we do this
			err = core.ErrorAsWriteError(err)
			if err != nil {
				return err
			}
			if m.G().Cfg().JSONOutput() {
				return JSONOutput(m, res)
			}
			did, err := res.KVNodeID().StringErr()
			if err != nil {
				return err
			}
			m.G().UIs().Terminal.Printf("DirID: %s\n", did)
			return PartingConsoleMessage(m)
		},
	)
}

func init() {
	AddCmd(kvCmd)
}
