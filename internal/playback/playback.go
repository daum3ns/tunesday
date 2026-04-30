package playback

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tunesday/internal/core"
)

type Player struct {
	ctx           context.Context
	cancel        context.CancelFunc
	cmd           *exec.Cmd
	socketPath    string
	conn          net.Conn
	mu            sync.Mutex
	tunes         []core.Tune
	currentIdx    int
	paused        bool
	debug         bool
	volume        int
	pollDone      chan struct{}
	onTrackChange func(int)
	onUpdate      func()
}

type ipcCommand struct {
	Command []interface{} `json:"command"`
}

func NewPlayer(tunes []core.Tune, debug ...bool) (*Player, error) {
	if err := checkDeps(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := false
	if len(debug) > 0 {
		d = debug[0]
	}
	return &Player{
		ctx:        ctx,
		cancel:     cancel,
		tunes:      tunes,
		currentIdx: 0,
		paused:     false,
		debug:      d,
		volume:     100,
	}, nil
}

func checkDeps() error {
	if _, err := exec.LookPath("mpv"); err != nil {
		return fmt.Errorf("mpv not found. Install: sudo apt install mpv")
	}
	return nil
}

func (p *Player) Start() error {
	p.socketPath = filepath.Join(os.TempDir(), fmt.Sprintf("tunesday-mpv-%d.sock", os.Getpid()))

	args := []string{
		"--no-video",
		"--input-ipc-server=" + p.socketPath,
		"--playlist=-",
		"--no-terminal",
		"--input-terminal=no",
	}

	// DEBUG MODE: Log what would be executed
	if p.debug {
		fmt.Println("[DEBUG] Would execute:")
		fmt.Printf("[DEBUG]   Command: mpv %s\n", strings.Join(args, " "))
		fmt.Println("[DEBUG]   URLs that would be piped to stdin:")
		for _, t := range p.tunes {
			fmt.Printf("[DEBUG]     - %s\n", t.Link)
		}
		fmt.Println("[DEBUG]   (Not actually starting mpv in debug mode)")
		return nil
	}

	p.cmd = exec.CommandContext(p.ctx, "mpv", args...)

	stdin, err := p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %w", err)
	}

	go func() {
		defer stdin.Close()
		for _, t := range p.tunes {
			fmt.Fprintln(stdin, t.Link)
		}
	}()

	go func() {
		<-p.ctx.Done()
		if p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
	}()

	return nil
}

func (p *Player) connectIPC() error {
	var err error
	for i := 0; i < 10; i++ {
		p.conn, err = net.Dial("unix", p.socketPath)
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("failed to connect to mpv IPC: %w", err)
}

func (p *Player) sendCommand(command []interface{}) error {
	// DEBUG MODE: Log the command instead of sending
	if p.debug {
		fmt.Printf("[DEBUG] Would send IPC command: %v\n", command)
		return nil
	}

	// Note: Caller must hold p.mu.Lock()
	if p.conn == nil {
		if err := p.connectIPC(); err != nil {
			return err
		}
	}

	cmd := ipcCommand{Command: command}
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}
	data = append(data, '\n')

	_, err = p.conn.Write(data)
	if err != nil {
		p.conn = nil
		return fmt.Errorf("failed to send command: %w", err)
	}
	return nil
}

type ipcResponse struct {
	Data  interface{} `json:"data"`
	Error string      `json:"error"`
}

func (p *Player) queryProperty(property string) (interface{}, error) {
	if p.debug {
		return nil, fmt.Errorf("query not supported in debug mode")
	}

	if p.conn == nil {
		if err := p.connectIPC(); err != nil {
			return nil, err
		}
	}

	// Send query command
	cmd := ipcCommand{Command: []interface{}{"get_property", property}}
	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}
	data = append(data, '\n')

	if _, err := p.conn.Write(data); err != nil {
		p.conn = nil
		return nil, fmt.Errorf("failed to send query: %w", err)
	}

	// Read response
	reader := bufio.NewReader(p.conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp ipcResponse
	if err := json.Unmarshal([]byte(response), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != "success" {
		return nil, fmt.Errorf("mpv error: %s", resp.Error)
	}

	return resp.Data, nil
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = true
	return p.sendCommand([]interface{}{"set_property", "pause", true})
}

func (p *Player) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = false
	return p.sendCommand([]interface{}{"set_property", "pause", false})
}

func (p *Player) TogglePause() error {
	if p.paused {
		return p.Resume()
	}
	return p.Pause()
}

func (p *Player) Next() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.currentIdx < len(p.tunes)-1 {
		p.currentIdx++
	}
	idx := p.currentIdx
	// Use set_property for reliable control
	return p.sendCommand([]interface{}{"set_property", "playlist-pos", idx})
}

func (p *Player) Prev() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.currentIdx > 0 {
		p.currentIdx--
	}
	idx := p.currentIdx
	// Use set_property for reliable control
	return p.sendCommand([]interface{}{"set_property", "playlist-pos", idx})
}

func (p *Player) Stop() {
	p.cancel()

	// DEBUG MODE: Just log
	if p.debug {
		fmt.Println("[DEBUG] Would stop mpv (not actually running)")
		return
	}

	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	os.Remove(p.socketPath)
}

func (p *Player) Wait() error {
	// DEBUG MODE: Wait for context cancellation instead of mpv process
	if p.debug {
		<-p.ctx.Done()
		return nil
	}
	// If cmd is nil (shouldn't happen in non-debug mode), return nil
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

func (p *Player) CurrentIndex() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentIdx
}

func (p *Player) GetPlaylistPos() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := p.queryProperty("playlist-pos")
	if err != nil {
		return 0, err
	}

	switch v := data.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected type for playlist-pos: %T", data)
	}
}

func (p *Player) GetTimePos() (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := p.queryProperty("time-pos")
	if err != nil {
		return 0, err
	}

	switch v := data.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("unexpected type for time-pos: %T", data)
	}
}

func (p *Player) GetDuration() (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := p.queryProperty("duration")
	if err != nil {
		return 0, err
	}

	switch v := data.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("unexpected type for duration: %T", data)
	}
}

func (p *Player) SetOnTrackChange(callback func(int)) {
	p.onTrackChange = callback
}

func (p *Player) SetOnUpdate(callback func()) {
	p.onUpdate = callback
}

func (p *Player) StartPolling(interval time.Duration) {
	p.pollDone = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var lastIdx int
		for {
			select {
			case <-p.pollDone:
				return
			case <-ticker.C:
				pos, err := p.GetPlaylistPos()
				if err != nil {
					goto UPDATE
				}
				p.mu.Lock()
				if pos != p.currentIdx {
					p.currentIdx = pos
					if p.onTrackChange != nil {
						p.onTrackChange(pos)
					}
				}
				if pos != lastIdx {
					lastIdx = pos
					if p.onTrackChange != nil {
						p.onTrackChange(pos)
					}
				}
				p.mu.Unlock()
			UPDATE:
				if p.onUpdate != nil {
					p.onUpdate()
				}
			}
		}
	}()
}

func (p *Player) StopPolling() {
	if p.pollDone != nil {
		close(p.pollDone)
	}
}

func (p *Player) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

const (
	minVolume  = 0
	maxVolume  = 100
	volumeStep = 10
)

func (p *Player) VolumeUp() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.volume < maxVolume {
		p.volume += volumeStep
		if p.volume > maxVolume {
			p.volume = maxVolume
		}
	}

	// Send to mpv
	return p.sendCommand([]interface{}{"set_property", "volume", p.volume})
}

func (p *Player) VolumeDown() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.volume > minVolume {
		p.volume -= volumeStep
		if p.volume < minVolume {
			p.volume = minVolume
		}
	}

	// Send to mpv
	return p.sendCommand([]interface{}{"set_property", "volume", p.volume})
}

func (p *Player) SetVolume(vol int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if vol < minVolume {
		vol = minVolume
	}
	if vol > maxVolume {
		vol = maxVolume
	}
	p.volume = vol

	// Send to mpv
	return p.sendCommand([]interface{}{"set_property", "volume", p.volume})
}

func (p *Player) GetVolume() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

func (p *Player) Tunes() []core.Tune {
	return p.tunes
}
