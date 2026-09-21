package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	netutil "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type page int

const (
	pageDashboard page = iota
	pageProcesses
	pageDocker
	pageNetwork
)

type metrics struct {
	hostname string
	osName   string
	kernel   string
	arch     string
	uptime   string

	cores int

	cpu    float64
	load1  float64
	load5  float64
	load15 float64

	memUsed  uint64
	memTotal uint64
	memPct   float64

	diskUsed  uint64
	diskTotal uint64
	diskPct   float64

	rxTotal uint64
	txTotal uint64
	rxRate  uint64
	txRate  uint64

	ufwInstalled        bool
	ufwActive           bool
	dockerInstalled     bool
	dockerActive        bool
	nginxInstalled      bool
	nginxActive         bool
	sshInstalled        bool
	sshActive           bool
	containerdInstalled bool
	containerdActive    bool
	gitInstalled        bool
	goInstalled         bool
	kubectlInstalled    bool
	kubeadmInstalled    bool
	ssInstalled         bool
	systemdInstalled    bool

	containers        int
	runningContainers int
	images            int

	updated string
}

type procInfo struct {
	pid     int32
	ppid    int32
	name    string
	user    string
	service string
	family  string
	risk    string
	cpu     float64
	mem     float32
}

type containerInfo struct {
	name   string
	image  string
	status string
	ports  string
}

type imageInfo struct {
	repo string
	tag  string
	id   string
	size string
}

type portInfo struct {
	proto string
	addr  string
	proc  string
	pid   string
}

type fwRule struct {
	port   string
	action string
	from   string
}

type model struct {
	m      metrics
	page   page
	width  int
	height int

	procs    []procInfo
	filtered []procInfo

	cursor   int
	selected map[int32]bool

	searchMode bool
	search     string
	matches    map[int32]bool
	related    map[int32]bool

	confirm     bool
	confirmText string

	killPIDs  []int32
	killNames map[int32]string
	killRisk  string

	containers []containerInfo
	images     []imageInfo

	ports []portInfo
	fw    []fwRule

	message string

	lastRX  uint64
	lastTX  uint64
	lastNet time.Time
}

type metricsMsg struct {
	m metrics
}

type procMsg struct {
	p []procInfo
}

type dockerMsg struct {
	containers []containerInfo
	images     []imageInfo
}

type networkMsg struct {
	ports []portInfo
	fw    []fwRule
}

type tickMsg time.Time

var (
	cyan = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00D7FF"))

	white = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF"))

	green = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00D787")).
		Bold(true)

	yellow = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD75F")).
		Bold(true)

	red = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF5F56")).
		Bold(true)

	muted = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#777777"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00BFFF")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#149BD7")).
			Padding(0, 1)
)

var familyColors = []lipgloss.Color{
	"#00D7FF",
	"#00D787",
	"#FFD75F",
	"#FF87FF",
	"#87D7FF",
	"#FF875F",
	"#AF87FF",
	"#5FD7AF",
	"#D7AF5F",
	"#87FF87",
	"#FF5F87",
	"#5FD7FF",
	"#D787FF",
	"#87FFFF",
}

func initialModel() model {
	return model{
		page:      pageDashboard,
		selected:  make(map[int32]bool),
		matches:   make(map[int32]bool),
		related:   make(map[int32]bool),
		killNames: make(map[int32]string),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		getMetrics,
		getProcesses,
		getDocker,
		getNetwork,
		tick(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height

	case tickMsg:
		cmds := []tea.Cmd{tick()}

		switch m.page {
		case pageDashboard:
			cmds = append(cmds, getMetrics)

		case pageProcesses:
			if !m.confirm && !m.searchMode {
				cmds = append(cmds, getProcesses)
			}

		case pageDocker:
			cmds = append(cmds, getDocker)

		case pageNetwork:
			cmds = append(cmds, getNetwork)
		}

		return m, tea.Batch(cmds...)

	case metricsMsg:
		v.m.rxRate, v.m.txRate = m.calculateNetworkRate(v.m.rxTotal, v.m.txTotal)
		m.m = v.m

	case procMsg:
		m.procs = v.p
		m.cleanupSelections()
		m.applyFilter()

	case dockerMsg:
		m.containers = v.containers
		m.images = v.images

	case networkMsg:
		m.ports = v.ports
		m.fw = v.fw

	case tea.KeyMsg:
		if m.confirm {
			return m.confirmKeys(v)
		}

		if m.searchMode {
			return m.searchKeys(v)
		}

		switch m.page {
		case pageProcesses:
			return m.processKeys(v)

		case pageDocker, pageNetwork:
			return m.pageKeys(v)

		default:
			return m.dashboardKeys(v)
		}
	}

	return m, nil
}

func (m *model) calculateNetworkRate(rx, tx uint64) (uint64, uint64) {
	now := time.Now()

	if m.lastNet.IsZero() {
		m.lastRX = rx
		m.lastTX = tx
		m.lastNet = now
		return 0, 0
	}

	seconds := now.Sub(m.lastNet).Seconds()

	if seconds <= 0 {
		return m.m.rxRate, m.m.txRate
	}

	var rxRate uint64
	var txRate uint64

	if rx >= m.lastRX {
		rxRate = uint64(float64(rx-m.lastRX) / seconds)
	}

	if tx >= m.lastTX {
		txRate = uint64(float64(tx-m.lastTX) / seconds)
	}

	m.lastRX = rx
	m.lastTX = tx
	m.lastNet = now

	return rxRate, txRate
}

func (m model) dashboardKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "p":
		m.page = pageProcesses
		return m, getProcesses

	case "d":
		m.page = pageDocker
		return m, getDocker

	case "n":
		m.page = pageNetwork
		return m, getNetwork

	case "r":
		return m, getMetrics
	}

	return m, nil
}

func (m model) pageKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.page = pageDashboard
		return m, getMetrics

	case "r":
		if m.page == pageDocker {
			return m, getDocker
		}
		return m, getNetwork

	case "p":
		m.page = pageProcesses
		return m, getProcesses

	case "d":
		m.page = pageDocker
		return m, getDocker

	case "n":
		m.page = pageNetwork
		return m, getNetwork
	}

	return m, nil
}

func (m model) processKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.page = pageDashboard
		m.searchMode = false
		m.search = ""
		m.matches = make(map[int32]bool)
		m.related = make(map[int32]bool)
		m.applyFilter()
		return m, getMetrics

	case "up", "k":
		m.moveCursor(-1)

	case "down", "j":
		m.moveCursor(1)

	case " ":
		return m.toggleSelection()

	case "/":
		m.searchMode = true
		m.search = ""
		m.cursor = 0
		m.buildSearch()
		m.applyFilter()
		m.message = "Type process/service/user • Enter Apply • Esc Cancel"

	case "backspace", "ctrl+h":
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.buildSearch()
			m.applyFilter()
		}

	case "r":
		m.message = "Refreshing processes..."
		return m, getProcesses

	case "K":
		return m.startKill()
	}

	return m, nil
}

func (m model) searchKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.searchMode = false
		m.search = ""
		m.matches = make(map[int32]bool)
		m.related = make(map[int32]bool)
		m.applyFilter()
		m.message = "Search cleared."

	case "enter":
		m.searchMode = false
		m.buildSearch()
		m.applyFilter()
		m.message = fmt.Sprintf(
			"Search: %q • direct matches grouped first.",
			m.search,
		)

	case "backspace", "ctrl+h":
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
		}

		m.buildSearch()
		m.applyFilter()

	case "ctrl+u":
		m.search = ""
		m.buildSearch()
		m.applyFilter()

	default:
		s := k.String()

		if len(s) == 1 && s[0] >= 32 && s[0] <= 126 {
			m.search += s
			m.buildSearch()
			m.applyFilter()
		}
	}

	return m, nil
}

func (m *model) moveCursor(delta int) {
	if len(m.filtered) == 0 {
		return
	}

	m.cursor += delta

	if m.cursor < 0 {
		m.cursor = 0
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}

	m.message = m.processWarning()
}

func (m model) toggleSelection() (tea.Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		m.message = "No process available."
		return m, nil
	}

	p := m.filtered[m.cursor]

	if p.risk == "DANGER" {
		m.message = fmt.Sprintf(
			"DANGER: PID %d (%s) is protected.",
			p.pid,
			p.name,
		)
		return m, nil
	}

	if m.selected[p.pid] {
		delete(m.selected, p.pid)
	} else {
		m.selected[p.pid] = true
	}

	m.message = m.processWarning()

	return m, nil
}

func (m *model) buildSearch() {
	m.matches = make(map[int32]bool)
	m.related = make(map[int32]bool)

	query := strings.ToLower(strings.TrimSpace(m.search))

	if query == "" {
		return
	}

	byPID := make(map[int32]procInfo)
	children := make(map[int32][]int32)

	for _, p := range m.procs {
		byPID[p.pid] = p
		children[p.ppid] = append(children[p.ppid], p.pid)

		if strings.Contains(strings.ToLower(p.name), query) ||
			strings.Contains(strings.ToLower(p.service), query) ||
			strings.Contains(strings.ToLower(p.user), query) {
			m.matches[p.pid] = true
		}
	}

	for pid := range m.matches {
		current := pid

		for {
			p, ok := byPID[current]

			if !ok || p.ppid == 0 {
				break
			}

			if !m.matches[p.ppid] {
				m.related[p.ppid] = true
			}

			current = p.ppid
		}
	}

	queue := make([]int32, 0, len(m.matches))

	for pid := range m.matches {
		queue = append(queue, pid)
	}

	visited := make(map[int32]bool)

	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]

		if visited[parent] {
			continue
		}

		visited[parent] = true

		for _, child := range children[parent] {
			if !m.matches[child] {
				m.related[child] = true
			}

			queue = append(queue, child)
		}
	}
}

func (m *model) applyFilter() {
	if strings.TrimSpace(m.search) == "" {
		m.filtered = append(m.filtered[:0], m.procs...)
		m.fixCursor()
		return
	}

	direct := []procInfo{}
	related := []procInfo{}
	others := []procInfo{}

	for _, p := range m.procs {
		if m.matches[p.pid] {
			direct = append(direct, p)
		} else if m.related[p.pid] {
			related = append(related, p)
		} else {
			others = append(others, p)
		}
	}

	sort.Slice(direct, func(i, j int) bool {
		return direct[i].pid < direct[j].pid
	})

	sort.Slice(related, func(i, j int) bool {
		return related[i].pid < related[j].pid
	})

	sort.Slice(others, func(i, j int) bool {
		return others[i].pid < others[j].pid
	})

	m.filtered = append(
		append(
			append([]procInfo{}, direct...),
			related...,
		),
		others...,
	)

	m.fixCursor()
}

func (m *model) fixCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}

	if m.cursor < 0 {
		m.cursor = 0
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *model) cleanupSelections() {
	present := make(map[int32]bool)

	for _, p := range m.procs {
		present[p.pid] = true
	}

	for pid := range m.selected {
		if !present[pid] {
			delete(m.selected, pid)
		}
	}
}

func (m model) processWarning() string {
	if len(m.filtered) == 0 {
		return "No matching processes found."
	}

	p := m.filtered[m.cursor]

	if p.risk == "DANGER" {
		return fmt.Sprintf(
			"DANGER: PID %d (%s) is protected.",
			p.pid,
			p.name,
		)
	}

	if p.risk == "CAUTION" {
		return fmt.Sprintf(
			"CAUTION: PID %d (%s) belongs to an important service.",
			p.pid,
			p.name,
		)
	}

	if m.selected[p.pid] {
		return fmt.Sprintf(
			"Selected PID %d (%s).",
			p.pid,
			p.name,
		)
	}

	return fmt.Sprintf(
		"PID %d (%s) • Family: %s",
		p.pid,
		p.name,
		p.family,
	)
}

func (m model) startKill() (tea.Model, tea.Cmd) {
	m.killPIDs = nil
	m.killNames = make(map[int32]string)
	m.killRisk = "SAFE"

	if len(m.selected) > 0 {
		for _, p := range m.procs {
			if !m.selected[p.pid] {
				continue
			}

			if p.risk == "DANGER" {
				m.message = fmt.Sprintf(
					"DANGER: PID %d (%s) is protected.",
					p.pid,
					p.name,
				)
				return m, nil
			}

			m.killPIDs = append(m.killPIDs, p.pid)
			m.killNames[p.pid] = p.name

			if p.risk == "CAUTION" {
				m.killRisk = "CAUTION"
			}
		}
	} else {
		if len(m.filtered) == 0 {
			m.message = "No process available."
			return m, nil
		}

		p := m.filtered[m.cursor]

		if p.risk == "DANGER" {
			m.message = fmt.Sprintf(
				"DANGER: PID %d (%s) is protected.",
				p.pid,
				p.name,
			)
			return m, nil
		}

		m.killPIDs = []int32{p.pid}
		m.killNames[p.pid] = p.name
		m.killRisk = p.risk
	}

	if len(m.killPIDs) == 0 {
		m.message = "No process selected."
		return m, nil
	}

	sort.Slice(m.killPIDs, func(i, j int) bool {
		return m.killPIDs[i] < m.killPIDs[j]
	})

	m.confirm = true
	m.confirmText = ""

	return m, nil
}

func (m model) confirmKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.confirm = false
		m.confirmText = ""
		m.killPIDs = nil
		m.message = "Cancelled. Nothing was terminated."
		return m, nil

	case "backspace", "ctrl+h":
		if len(m.confirmText) > 0 {
			m.confirmText = m.confirmText[:len(m.confirmText)-1]
		}
		return m, nil

	case "enter":
		if strings.ToUpper(strings.TrimSpace(m.confirmText)) != "CONFIRMED" {
			m.message = "Type CONFIRMED exactly, then press Enter."
			return m, nil
		}

		success := 0
		failed := 0

		for _, pid := range m.killPIDs {
			if err := terminateVerified(pid, m.killNames[pid]); err != nil {
				failed++
			} else {
				success++
			}
		}

		m.confirm = false
		m.confirmText = ""
		m.killPIDs = nil
		m.killNames = make(map[int32]string)
		m.selected = make(map[int32]bool)

		m.message = fmt.Sprintf(
			"Termination complete: %d succeeded, %d failed. Refreshing...",
			success,
			failed,
		)

		return m, getProcesses

	default:
		s := k.String()

		if len(s) == 1 && s[0] >= 32 && s[0] <= 126 {
			m.confirmText += s
		}

		return m, nil
	}
}

func terminateVerified(pid int32, expected string) error {
	p, err := process.NewProcess(pid)

	if err != nil {
		return err
	}

	name, err := p.Name()

	if err != nil {
		return err
	}

	if name != expected {
		return fmt.Errorf("PID reused")
	}

	if processRisk(name, serviceForPID(pid)) == "DANGER" {
		return fmt.Errorf("protected process")
	}

	return p.Kill()
}

func getProcesses() tea.Msg {
	return procMsg{
		p: collectProcesses(),
	}
}

func collectProcesses() []procInfo {
	list, err := process.Processes()

	if err != nil {
		return nil
	}

	result := make([]procInfo, 0, len(list))

	for _, p := range list {
		name, err := p.Name()

		if err != nil {
			continue
		}

		ppid, _ := p.Ppid()
		user, _ := p.Username()
		cpuPercent, _ := p.CPUPercent()
		memPercent, _ := p.MemoryPercent()

		service := serviceForPID(p.Pid)

		result = append(
			result,
			procInfo{
				pid:     p.Pid,
				ppid:    ppid,
				name:    name,
				user:    user,
				service: service,
				family:  processFamily(name),
				risk:    processRisk(name, service),
				cpu:     cpuPercent,
				mem:     memPercent,
			},
		)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].pid < result[j].pid
	})

	return result
}

func processFamily(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))

	families := []string{
		"kworker",
		"rcu",
		"migration",
		"cpuhp",
		"idle_inject",
		"watchdog",
		"systemd",
		"sshd",
		"nginx",
		"docker",
		"containerd",
		"mysql",
		"postgres",
		"redis",
		"python",
		"node",
		"java",
		"php",
		"bash",
		"zsh",
		"cron",
	}

	for _, family := range families {
		if strings.HasPrefix(name, family) {
			return family
		}
	}

	for _, separator := range []string{"/", ":", "-"} {
		if i := strings.Index(name, separator); i > 0 {
			return name[:i]
		}
	}

	if name == "" {
		return "unknown"
	}

	return name
}

func processRisk(name, service string) string {
	name = strings.ToLower(name)
	service = strings.ToLower(service)

	if criticalProcess(name) ||
		strings.Contains(service, "ssh") {
		return "DANGER"
	}

	caution := []string{
		"docker",
		"containerd",
		"nginx",
		"mysql",
		"mysqld",
		"postgres",
		"postgresql",
		"redis",
		"redis-server",
		"apache2",
	}

	for _, item := range caution {
		if strings.Contains(name, item) {
			return "CAUTION"
		}
	}

	return "SAFE"
}

func criticalProcess(name string) bool {
	switch name {
	case "systemd",
		"init",
		"sshd",
		"systemd-journald",
		"systemd-logind",
		"systemd-udevd":
		return true
	}

	return false
}

func serviceForPID(pid int32) string {
	path := "/proc/" +
		strconv.Itoa(int(pid)) +
		"/cgroup"

	data, err := os.ReadFile(path)

	if err != nil {
		return "-"
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, ".service") {
			continue
		}

		parts := strings.Split(line, "/")

		for i := len(parts) - 1; i >= 0; i-- {
			if strings.HasSuffix(parts[i], ".service") {
				return strings.TrimSpace(parts[i])
			}
		}
	}

	return "-"
}

func getMetrics() tea.Msg {
	var m metrics

	m.updated = time.Now().Format("15:04:05")

	m.hostname, _ = os.Hostname()
	m.arch = runtime.GOARCH

	if h, err := host.Info(); err == nil {
		m.osName = h.Platform + " " + h.PlatformVersion
		m.kernel = h.KernelVersion
		m.uptime = formatUptime(h.Uptime)
	}

	m.cores, _ = cpu.Counts(true)

	if values, err := cpu.Percent(100*time.Millisecond, false); err == nil &&
		len(values) > 0 {
		m.cpu = values[0]
	}

	if l, err := load.Avg(); err == nil {
		m.load1 = l.Load1
		m.load5 = l.Load5
		m.load15 = l.Load15
	}

	if v, err := mem.VirtualMemory(); err == nil {
		m.memUsed = v.Used
		m.memTotal = v.Total
		m.memPct = v.UsedPercent
	}

	if d, err := disk.Usage("/"); err == nil {
		m.diskUsed = d.Used
		m.diskTotal = d.Total
		m.diskPct = d.UsedPercent
	}

	if io, err := netutil.IOCounters(false); err == nil {
		for _, iface := range io {
			m.rxTotal += iface.BytesRecv
			m.txTotal += iface.BytesSent
		}
	}

	m.ufwInstalled = exists("ufw")
	m.ufwActive = m.ufwInstalled && ufwActive()

	m.dockerInstalled = exists("docker")
	m.dockerActive = m.dockerInstalled && serviceActive("docker")

	m.nginxInstalled = exists("nginx")
	m.nginxActive = m.nginxInstalled && serviceActive("nginx")

	m.sshInstalled = exists("sshd") || exists("ssh")
	m.sshActive = serviceActive("ssh") || serviceActive("sshd")

	m.containerdInstalled = exists("containerd")
	m.containerdActive = m.containerdInstalled && serviceActive("containerd")

	m.gitInstalled = exists("git")
	m.goInstalled = exists("go")
	m.kubectlInstalled = exists("kubectl")
	m.kubeadmInstalled = exists("kubeadm")
	m.ssInstalled = exists("ss")
	m.systemdInstalled = exists("systemctl")

	m.containers, m.runningContainers, m.images = dockerCounts()

	return metricsMsg{m: m}
}

func formatUptime(seconds uint64) string {
	days := seconds / 86400
	seconds %= 86400

	hours := seconds / 3600
	seconds %= 3600

	minutes := seconds / 60

	if days > 0 {
		return fmt.Sprintf(
			"%dd %dh %dm",
			days,
			hours,
			minutes,
		)
	}

	return fmt.Sprintf(
		"%dh %dm",
		hours,
		minutes,
	)
}

func exists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func serviceActive(service string) bool {
	output, err := exec.Command(
		"systemctl",
		"is-active",
		service,
	).Output()

	return err == nil &&
		strings.TrimSpace(string(output)) == "active"
}

func ufwActive() bool {
	output, err := exec.Command(
		"ufw",
		"status",
	).Output()

	return err == nil &&
		strings.Contains(string(output), "Status: active")
}

func formatBytes(value uint64) string {
	units := []string{
		"B",
		"KB",
		"MB",
		"GB",
		"TB",
	}

	f := float64(value)
	index := 0

	for f >= 1024 && index < len(units)-1 {
		f /= 1024
		index++
	}

	if index == 0 {
		return fmt.Sprintf(
			"%d %s",
			value,
			units[index],
		)
	}

	return fmt.Sprintf(
		"%.1f %s",
		f,
		units[index],
	)
}

func formatRate(value uint64) string {
	return fmt.Sprintf(
		"%.1f KB/s",
		float64(value)/1024,
	)
}

func progress(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}

	if percent > 100 {
		percent = 100
	}

	filled := int(
		percent / 100 * float64(width),
	)

	return strings.Repeat("█", filled) +
		strings.Repeat("░", width-filled)
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}

	if max <= 3 {
		return text[:max]
	}

	return text[:max-3] + "..."
}

func dockerCounts() (int, int, int) {
	if !exists("docker") {
		return 0, 0, 0
	}

	containers := getContainers()
	running := 0

	for _, c := range containers {
		if strings.HasPrefix(c.status, "Up ") {
			running++
		}
	}

	images := getImages()

	return len(containers),
		running,
		len(images)
}

func getDocker() tea.Msg {
	return dockerMsg{
		containers: getContainers(),
		images:     getImages(),
	}
}

func getContainers() []containerInfo {
	if !exists("docker") {
		return nil
	}

	output, err := exec.Command(
		"docker",
		"ps",
		"-a",
		"--format",
		"{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}",
	).Output()

	if err != nil {
		return nil
	}

	result := []containerInfo{}

	for _, line := range strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	) {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		for len(fields) < 4 {
			fields = append(fields, "-")
		}

		result = append(
			result,
			containerInfo{
				name:   fields[0],
				image:  fields[1],
				status: fields[2],
				ports:  fields[3],
			},
		)
	}

	return result
}

func getImages() []imageInfo {
	if !exists("docker") {
		return nil
	}

	output, err := exec.Command(
		"docker",
		"images",
		"--format",
		"{{.Repository}}\t{{.Tag}}\t{{.ID}}\t{{.Size}}",
	).Output()

	if err != nil {
		return nil
	}

	result := []imageInfo{}

	for _, line := range strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	) {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		for len(fields) < 4 {
			fields = append(fields, "-")
		}

		result = append(
			result,
			imageInfo{
				repo: fields[0],
				tag:  fields[1],
				id:   fields[2],
				size: fields[3],
			},
		)
	}

	return result
}

func getNetwork() tea.Msg {
	return networkMsg{
		ports: listenPorts(),
		fw:    getUFWRules(),
	}
}

func listenPorts() []portInfo {
	if !exists("ss") {
		return nil
	}

	output, err := exec.Command(
		"ss",
		"-H",
		"-lntuop",
	).Output()

	if err != nil {
		return nil
	}

	result := []portInfo{}

	for _, line := range strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	) {
		fields := strings.Fields(line)

		if len(fields) < 5 {
			continue
		}

		item := portInfo{
			proto: fields[0],
			addr:  fields[4],
			proc:  "-",
			pid:   "-",
		}

		full := strings.Join(fields, " ")

		if index := strings.Index(full, "users:"); index >= 0 {
			data := full[index:]

			if start := strings.Index(data, `("`); start >= 0 {
				data = data[start+2:]

				if end := strings.Index(data, `"`); end >= 0 {
					item.proc = data[:end]
				}
			}

			if start := strings.Index(data, "pid="); start >= 0 {
				data = data[start+4:]
				end := len(data)

				for i, r := range data {
					if r < '0' || r > '9' {
						end = i
						break
					}
				}

				item.pid = data[:end]
			}
		}

		result = append(result, item)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].addr < result[j].addr
	})

	return result
}

func getUFWRules() []fwRule {
	if !exists("ufw") {
		return nil
	}

	output, err := exec.Command(
		"ufw",
		"status",
	).Output()

	if err != nil {
		return nil
	}

	result := []fwRule{}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)

		if line == "" ||
			strings.HasPrefix(line, "Status:") ||
			strings.HasPrefix(line, "To") ||
			strings.HasPrefix(line, "--") {
			continue
		}

		fields := strings.Fields(line)

		if len(fields) < 2 {
			continue
		}

		from := "-"

		if len(fields) > 2 {
			from = strings.Join(fields[2:], " ")
		}

		result = append(
			result,
			fwRule{
				port:   fields[0],
				action: fields[1],
				from:   from,
			},
		)
	}

	return result
}

func familyStyle(family string) lipgloss.Style {
	var hash uint32

	for i := 0; i < len(family); i++ {
		hash = hash*31 + uint32(family[i])
	}

	index := int(
		hash % uint32(len(familyColors)),
	)

	return lipgloss.NewStyle().
		Foreground(familyColors[index])
}

func title(text string) string {
	return cyan.Bold(true).Render(text)
}

func serviceState(
	installed bool,
	active bool,
	isService bool,
) string {
	if !installed {
		return red.Render("● NOT INSTALLED")
	}

	if isService {
		if active {
			return green.Render("● ACTIVE")
		}

		return red.Render("● INACTIVE")
	}

	return yellow.Render("● INSTALLED")
}

func (m model) View() string {
	if m.width == 0 {
		return "Starting ServerGuard..."
	}

	switch m.page {
	case pageProcesses:
		return m.processView()

	case pageDocker:
		return m.dockerView()

	case pageNetwork:
		return m.networkView()
	}

	return m.dashboardView()
}

func (m model) dashboardView() string {
	if m.width < 82 {
		return m.compactDashboard()
	}

	usable := m.width - 4
	left := usable / 2
	right := usable - left - 2

	if left < 32 || right < 32 {
		return m.compactDashboard()
	}

	card := func(width int, content string) string {
		return boxStyle.Width(width).Render(content)
	}

	system := card(
		left,
		fmt.Sprintf(
			"%s\n"+
				"Hostname: %s\n"+
				"OS: %s\n"+
				"Kernel: %s\n"+
				"Arch: %s\n"+
				"Uptime: %s\n"+
				"CPU Cores: %d",
			headerStyle.Render("SYSTEM"),
			m.m.hostname,
			m.m.osName,
			m.m.kernel,
			m.m.arch,
			m.m.uptime,
			m.m.cores,
		),
	)

	cpuBox := card(
		right,
		fmt.Sprintf(
			"%s\n"+
				"%s %.1f%%\n"+
				"Load: %.2f %.2f %.2f\n"+
				"Cores: %d",
			headerStyle.Render("CPU"),
			progress(m.m.cpu, 20),
			m.m.cpu,
			m.m.load1,
			m.m.load5,
			m.m.load15,
			m.m.cores,
		),
	)

	memory := card(
		left,
		fmt.Sprintf(
			"%s\n"+
				"%s %.1f%%\n"+
				"Used: %s\n"+
				"Total: %s",
			headerStyle.Render("MEMORY"),
			progress(m.m.memPct, 20),
			m.m.memPct,
			formatBytes(m.m.memUsed),
			formatBytes(m.m.memTotal),
		),
	)

	diskBox := card(
		right,
		fmt.Sprintf(
			"%s\n"+
				"%s %.1f%%\n"+
				"Used: %s\n"+
				"Total: %s",
			headerStyle.Render("DISK /"),
			progress(m.m.diskPct, 20),
			m.m.diskPct,
			formatBytes(m.m.diskUsed),
			formatBytes(m.m.diskTotal),
		),
	)

	network := card(
		left,
		fmt.Sprintf(
			"%s\n"+
				"↓ Download: %s\n"+
				"↑ Upload:   %s",
			headerStyle.Render("NETWORK"),
			formatRate(m.m.rxRate),
			formatRate(m.m.txRate),
		),
	)

	docker := card(
		right,
		fmt.Sprintf(
			"%s\n"+
				"%s\n"+
				"Containers: %d\n"+
				"Running: %d\n"+
				"Images: %d",
			headerStyle.Render("DOCKER"),
			serviceState(
				m.m.dockerInstalled,
				m.m.dockerActive,
				true,
			),
			m.m.containers,
			m.m.runningContainers,
			m.m.images,
		),
	)

	top := strings.Join(topProcessLines(4), "\n")

	topProcesses := card(
		left,
		headerStyle.Render("TOP PROCESSES")+
			"\n"+
			"PID      PROCESS             CPU    MEM\n"+
			top,
	)

	services := card(
		right,
		fmt.Sprintf(
			"%s\n"+
				"SSH        %s\n"+
				"Docker     %s\n"+
				"Nginx      %s\n"+
				"containerd %s\n"+
				"UFW        %s",
			headerStyle.Render("CORE SERVICES"),
			serviceState(m.m.sshInstalled, m.m.sshActive, true),
			serviceState(m.m.dockerInstalled, m.m.dockerActive, true),
			serviceState(m.m.nginxInstalled, m.m.nginxActive, true),
			serviceState(m.m.containerdInstalled, m.m.containerdActive, true),
			serviceState(m.m.ufwInstalled, m.m.ufwActive, true),
		),
	)

	tools := boxStyle.
		Width(m.width - 4).
		Render(
			headerStyle.Render("ADMIN / DEVOPS") +
				"\n" +
				m.toolStatusText(m.width),
		)

	row1 := lipgloss.JoinHorizontal(
		lipgloss.Top,
		system,
		cpuBox,
	)

	row2 := lipgloss.JoinHorizontal(
		lipgloss.Top,
		memory,
		diskBox,
	)

	row3 := lipgloss.JoinHorizontal(
		lipgloss.Top,
		network,
		docker,
	)

	row4 := lipgloss.JoinHorizontal(
		lipgloss.Top,
		topProcesses,
		services,
	)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title("SERVERGUARD"),
		row1,
		row2,
		row3,
		row4,
		tools,
		muted.Render(
			"P Processes • D Docker • N Network • R Refresh • Q Quit • Updated "+
				m.m.updated,
		),
	)
}

func (m model) compactDashboard() string {
	width := m.width - 4

	if width < 20 {
		width = 20
	}

	card := func(content string) string {
		return boxStyle.Width(width).Render(content)
	}

	top := strings.Join(topProcessLines(3), "\n")

	return lipgloss.JoinVertical(
		lipgloss.Left,

		title("SERVERGUARD"),

		card(
			fmt.Sprintf(
				"%s\n"+
					"Host: %s\n"+
					"OS: %s\n"+
					"Kernel: %s\n"+
					"Uptime: %s • Cores: %d",
				headerStyle.Render("SYSTEM"),
				m.m.hostname,
				m.m.osName,
				m.m.kernel,
				m.m.uptime,
				m.m.cores,
			),
		),

		card(
			fmt.Sprintf(
				"%s\n"+
					"%s %.1f%%\n"+
					"Load: %.2f %.2f %.2f",
				headerStyle.Render("CPU"),
				progress(m.m.cpu, 16),
				m.m.cpu,
				m.m.load1,
				m.m.load5,
				m.m.load15,
			),
		),

		card(
			fmt.Sprintf(
				"%s\n"+
					"%s %.1f%%\n"+
					"Used: %s / %s",
				headerStyle.Render("MEMORY"),
				progress(m.m.memPct, 16),
				m.m.memPct,
				formatBytes(m.m.memUsed),
				formatBytes(m.m.memTotal),
			),
		),

		card(
			fmt.Sprintf(
				"%s\n"+
					"%s %.1f%%\n"+
					"Used: %s / %s",
				headerStyle.Render("DISK /"),
				progress(m.m.diskPct, 16),
				m.m.diskPct,
				formatBytes(m.m.diskUsed),
				formatBytes(m.m.diskTotal),
			),
		),

		card(
			fmt.Sprintf(
				"%s\n"+
					"↓ Download: %s\n"+
					"↑ Upload:   %s",
				headerStyle.Render("NETWORK"),
				formatRate(m.m.rxRate),
				formatRate(m.m.txRate),
			),
		),

		card(
			fmt.Sprintf(
				"%s\n"+
					"%s\n"+
					"Containers: %d • Running: %d • Images: %d",
				headerStyle.Render("DOCKER"),
				serviceState(
					m.m.dockerInstalled,
					m.m.dockerActive,
					true,
				),
				m.m.containers,
				m.m.runningContainers,
				m.m.images,
			),
		),

		card(
			headerStyle.Render("TOP PROCESSES")+
				"\n"+
				"PID      PROCESS             CPU    MEM\n"+
				top,
		),

		card(
			fmt.Sprintf(
				"%s\n"+
					"SSH        %s\n"+
					"Docker     %s\n"+
					"Nginx      %s\n"+
					"containerd %s\n"+
					"UFW        %s",
				headerStyle.Render("CORE SERVICES"),
				serviceState(m.m.sshInstalled, m.m.sshActive, true),
				serviceState(m.m.dockerInstalled, m.m.dockerActive, true),
				serviceState(m.m.nginxInstalled, m.m.nginxActive, true),
				serviceState(m.m.containerdInstalled, m.m.containerdActive, true),
				serviceState(m.m.ufwInstalled, m.m.ufwActive, true),
			),
		),

		boxStyle.
			Width(width).
			Render(
				headerStyle.Render("ADMIN / DEVOPS")+
					"\n"+
					m.toolStatusText(m.width),
			),

		muted.Render(
			"P Processes • D Docker • N Network • R Refresh • Q Quit • Updated "+
				m.m.updated,
		),
	)
}

func (m model) toolStatusText(width int) string {
	items := []struct {
		name      string
		installed bool
		active    bool
		service   bool
	}{
		{"UFW", m.m.ufwInstalled, m.m.ufwActive, true},
		{"Docker", m.m.dockerInstalled, m.m.dockerActive, true},
		{"containerd", m.m.containerdInstalled, m.m.containerdActive, true},
		{"Nginx", m.m.nginxInstalled, m.m.nginxActive, true},
		{"OpenSSH", m.m.sshInstalled, m.m.sshActive, true},
		{"systemd", m.m.systemdInstalled, true, true},
		{"Git", m.m.gitInstalled, false, false},
		{"Go", m.m.goInstalled, false, false},
		{"kubectl", m.m.kubectlInstalled, false, false},
		{"kubeadm", m.m.kubeadmInstalled, false, false},
		{"ss", m.m.ssInstalled, false, false},
	}

	columns := 4

	if width < 110 {
		columns = 3
	}

	if width < 82 {
		columns = 2
	}

	if width < 55 {
		columns = 1
	}

	var b strings.Builder

	for i, item := range items {
		b.WriteString(
			fmt.Sprintf(
				"%-12s %s",
				item.name,
				serviceState(
					item.installed,
					item.active,
					item.service,
				),
			),
		)

		if (i+1)%columns == 0 {
			b.WriteByte('\n')
		} else {
			b.WriteString("   ")
		}
	}

	return strings.TrimRight(
		b.String(),
		" \n",
	)
}

func (m model) processView() string {
	header := headerStyle.Render("PROCESS MANAGER") +
		"\nSearch: " +
		m.search

	if m.searchMode {
		header += "  [typing]"
	}

	table := fmt.Sprintf(
		"%-4s %-7s %-20s %-10s %6s %6s %-9s %-16s\n",
		"SEL",
		"PID",
		"PROCESS",
		"USER",
		"CPU",
		"MEM",
		"RISK",
		"SERVICE",
	)

	table += strings.Repeat("─", 110) + "\n"

	rows := m.height - 15

	if rows < 6 {
		rows = 6
	}

	start := 0

	if m.cursor >= rows {
		start = m.cursor - rows + 1
	}

	end := start + rows

	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := start; i < end; i++ {
		p := m.filtered[i]

		checkbox := "[ ]"

		if m.selected[p.pid] {
			checkbox = "[x]"
		}

		cursor := " "

		if i == m.cursor {
			cursor = ">"
		}

		line := fmt.Sprintf(
			"%s%-3s %-7d %-20s %-10s %6.1f %5.1f%% %-9s %-16s",
			cursor,
			checkbox,
			p.pid,
			truncate(p.name, 20),
			truncate(p.user, 10),
			p.cpu,
			p.mem,
			p.risk,
			truncate(p.service, 16),
		)

		style := familyStyle(p.family)

		if p.risk == "DANGER" {
			line = red.Render(line)
		} else if i == m.cursor {
			line = white.Bold(true).Render(line)
		} else if m.selected[p.pid] {
			line = green.Render(line)
		} else {
			if m.matches[p.pid] {
				style = style.Bold(true)
			}

			line = style.Render(line)
		}

		table += line + "\n"
	}

	info := fmt.Sprintf(
		"Found: %d   Selected: %d",
		len(m.filtered),
		len(m.selected),
	)

	if m.search != "" {
		info += fmt.Sprintf(
			"   Direct: %d   Related: %d",
			len(m.matches),
			len(m.related),
		)
	}

	if m.message != "" {
		if strings.Contains(m.message, "DANGER") {
			info += "\n" + red.Render(m.message)
		} else if strings.Contains(m.message, "CAUTION") {
			info += "\n" + yellow.Render(m.message)
		} else {
			info += "\n" + muted.Render(m.message)
		}
	}

	if m.confirm {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			title("SERVERGUARD / PROCESS MANAGER"),
			boxStyle.Render(header),
			boxStyle.Render(table+info),
			m.confirmView(),
			m.processFooter(),
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title("SERVERGUARD / PROCESS MANAGER"),
		boxStyle.Render(header),
		boxStyle.Render(table+info),
		m.processFooter(),
	)
}

func (m model) confirmView() string {
	list := ""

	for _, pid := range m.killPIDs {
		list += fmt.Sprintf(
			"PID %-8d %s\n",
			pid,
			m.killNames[pid],
		)
	}

	warning := "⚠ This will terminate the selected process(es)."

	if m.killRisk == "CAUTION" {
		warning =
			"⚠ CAUTION: This process belongs to an important service. " +
				"It may stop or affect that service."
	}

	return boxStyle.Render(
		headerStyle.Render(
			"CONFIRM PROCESS TERMINATION",
		) +
			"\n\nSelected:\n" +
			list +
			"\n" +
			warning +
			"\n\nType CONFIRMED to proceed:\n> " +
			m.confirmText +
			"_" +
			"\n\nENTER = Confirm Kill\n" +
			"ESC = Cancel\n" +
			"K = Open Kill Dialog",
	)
}

func (m model) processFooter() string {
	return muted.Render(
		"↑↓/j/k Navigate • Space Select • / Search • Backspace Delete • CAPITAL K Kill • R Refresh • D Docker • N Network • Esc Back • Q Quit",
	)
}

func (m model) dockerView() string {
	containerText :=
		headerStyle.Render("CONTAINERS") +
			"\n\n" +
			"NAME                     IMAGE                        STATUS                         PORT MAPPINGS\n" +
			strings.Repeat("─", 115) +
			"\n"

	for _, c := range m.containers {
		statusStyle := white

		if strings.HasPrefix(c.status, "Up ") {
			statusStyle = green
		}

		containerText += fmt.Sprintf(
			"%-24s %-28s %-30s %s\n",
			truncate(c.name, 24),
			truncate(c.image, 28),
			statusStyle.Render(truncate(c.status, 30)),
			truncate(c.ports, 32),
		)
	}

	if len(m.containers) == 0 {
		containerText += "No containers found.\n"
	}

	imageText :=
		headerStyle.Render("DOCKER IMAGES") +
			"\n\n" +
			"REPOSITORY                       TAG             IMAGE ID          SIZE\n" +
			strings.Repeat("─", 82) +
			"\n"

	for _, image := range m.images {
		imageText += fmt.Sprintf(
			"%-32s %-15s %-18s %-10s\n",
			truncate(image.repo, 32),
			truncate(image.tag, 15),
			truncate(image.id, 18),
			image.size,
		)
	}

	if len(m.images) == 0 {
		imageText += "No images found.\n"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title("SERVERGUARD / DOCKER"),
		boxStyle.Render(containerText),
		boxStyle.Render(imageText),
		muted.Render(
			"R Refresh • P Processes • N Network • Esc Dashboard • Q Quit",
		),
	)
}

func (m model) networkView() string {
	portText :=
		headerStyle.Render("LISTENING PORTS • ss -lntuop") +
			"\n\n" +
			"PROTO    LOCAL ADDRESS                       PROCESS                   PID\n" +
			strings.Repeat("─", 88) +
			"\n"

	for _, p := range m.ports {
		portText += fmt.Sprintf(
			"%-8s %-36s %-25s %-10s\n",
			p.proto,
			truncate(p.addr, 36),
			truncate(p.proc, 25),
			p.pid,
		)
	}

	if len(m.ports) == 0 {
		portText += "No listening ports found.\n"
	}

	firewallText :=
		headerStyle.Render("UFW FIREWALL") +
			"\n\n" +
			"PORT / PROTO          ACTION          FROM\n" +
			strings.Repeat("─", 82) +
			"\n"

	if !m.m.ufwInstalled {
		firewallText += "UFW is not installed.\n"
	} else if len(m.fw) == 0 {
		firewallText += "No UFW rules returned.\n"
	} else {
		for _, rule := range m.fw {
			actionStyle := white

			if strings.EqualFold(rule.action, "ALLOW") {
				actionStyle = green
			}

			if strings.EqualFold(rule.action, "DENY") ||
				strings.EqualFold(rule.action, "REJECT") {
				actionStyle = red
			}

			firewallText += fmt.Sprintf(
				"%-22s %-15s %-45s\n",
				rule.port,
				actionStyle.Render(rule.action),
				truncate(rule.from, 45),
			)
		}
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title("SERVERGUARD / NETWORK & FIREWALL"),
		boxStyle.Render(portText),
		boxStyle.Render(firewallText),
		muted.Render(
			"R Refresh • D Docker • P Processes • Esc Dashboard • Q Quit",
		),
	)
}

func topProcessLines(count int) []string {
	output, err := exec.Command(
		"ps",
		"-eo",
		"pid,comm,%cpu,%mem",
		"--sort=-%cpu",
	).Output()

	if err != nil {
		return []string{"Unable to read processes"}
	}

	lines := strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	)

	if len(lines) < 2 {
		return nil
	}

	if count > len(lines)-1 {
		count = len(lines) - 1
	}

	result := []string{}

	for _, line := range lines[1 : count+1] {
		fields := strings.Fields(line)

		if len(fields) >= 4 {
			result = append(
				result,
				fmt.Sprintf(
					"%-8s %-19s %5s%% %5s%%",
					fields[0],
					truncate(fields[1], 19),
					fields[2],
					fields[3],
				),
			)
		}
	}

	return result
}

func main() {
	program := tea.NewProgram(
		initialModel(),
		tea.WithAltScreen(),
	)

	if _, err := program.Run(); err != nil {
		fmt.Println("ServerGuard error:", err)
		os.Exit(1)
	}
}
