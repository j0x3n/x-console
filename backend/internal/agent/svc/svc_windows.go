//go:build windows

package svc

import (
	"context"
	"errors"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	winsvc "golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func available() bool { return true }

// openSCM connects with the least access needed. mgr.Connect asks for full
// access, which a normal user (the desktop agent) does not have.
func openSCM(access uint32) (*mgr.Mgr, error) {
	h, err := windows.OpenSCManager(nil, nil, access)
	if err != nil {
		return nil, rpcutil.FromOS(err)
	}
	return &mgr.Mgr{Handle: h}, nil
}

func openService(m *mgr.Mgr, name string, access uint32) (*mgr.Service, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, rpcutil.BadParams("invalid service name")
	}
	h, err := windows.OpenService(m.Handle, p, access)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil, &protocol.Error{Code: protocol.CodeNotFound, Message: "service " + name + " does not exist"}
		}
		return nil, rpcutil.FromOS(err)
	}
	return &mgr.Service{Name: name, Handle: h}, nil
}

func list(ctx context.Context) ([]protocol.ServiceInfo, error) {
	m, err := openSCM(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, err
	}
	defer m.Disconnect()

	var needed, returned uint32
	var buf []byte
	for {
		var p *byte
		if len(buf) > 0 {
			p = &buf[0]
		}
		err := windows.EnumServicesStatusEx(m.Handle, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32,
			windows.SERVICE_STATE_ALL, p, uint32(len(buf)), &needed, &returned, nil, nil)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.ERROR_MORE_DATA) || needed <= uint32(len(buf)) {
			return nil, rpcutil.FromOS(err)
		}
		buf = make([]byte, needed)
	}
	if returned == 0 {
		return []protocol.ServiceInfo{}, nil
	}
	entries := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), int(returned))
	items := make([]protocol.ServiceInfo, 0, len(entries))
	for _, e := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		name := windows.UTF16PtrToString(e.ServiceName)
		state, sub := WindowsState(e.ServiceStatusProcess.CurrentState)
		info := protocol.ServiceInfo{Name: name, Description: windows.UTF16PtrToString(e.DisplayName), State: state, SubState: sub}
		if s, err := openService(m, name, windows.SERVICE_QUERY_CONFIG); err == nil {
			if cfg, err := s.Config(); err == nil {
				info.StartType, info.Enabled = WindowsStartType(cfg.StartType, cfg.DelayedAutoStart)
			}
			s.Close()
		}
		items = append(items, info)
	}
	return items, nil
}

func action(ctx context.Context, name, act string) error {
	m, err := openSCM(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer m.Disconnect()
	switch act {
	case protocol.SvcStart:
		return start(m, name)
	case protocol.SvcStop:
		return stop(ctx, m, name)
	case protocol.SvcRestart:
		if err := stop(ctx, m, name); err != nil {
			return err
		}
		return start(m, name)
	case protocol.SvcEnable, protocol.SvcDisable:
		s, err := openService(m, name, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_CHANGE_CONFIG)
		if err != nil {
			return err
		}
		defer s.Close()
		cfg, err := s.Config()
		if err != nil {
			return rpcutil.FromOS(err)
		}
		if act == protocol.SvcEnable {
			cfg.StartType = mgr.StartAutomatic
		} else {
			cfg.StartType = mgr.StartDisabled
		}
		return rpcutil.FromOS(s.UpdateConfig(cfg))
	}
	return rpcutil.BadParams("unknown action")
}

func start(m *mgr.Mgr, name string) error {
	s, err := openService(m, name, windows.SERVICE_START|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return rpcutil.FromOS(err)
	}
	return nil
}

func stop(ctx context.Context, m *mgr.Mgr, name string) error {
	s, err := openService(m, name, windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return err
	}
	defer s.Close()
	st, err := s.Control(winsvc.Stop)
	if err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return rpcutil.FromOS(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != winsvc.Stopped {
		if time.Now().After(deadline) {
			return &protocol.Error{Code: protocol.CodeTimeout, Message: "service did not stop in 30 seconds"}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		if st, err = s.Query(); err != nil {
			return rpcutil.FromOS(err)
		}
	}
	return nil
}

// Reading the Windows event log is not implemented yet.
func logs(context.Context, string, int) ([]string, error) {
	return nil, rpcutil.Unsupported("service logs")
}
