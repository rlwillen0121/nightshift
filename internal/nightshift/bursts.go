package nightshift

import (
	"fmt"
	"sort"
	"strings"
)

// Hosts below end in .invalid. IPv4 addresses stay in the documentation
// ranges and IPv6 in 2001:db8::/32. Findings and seals are invented.
// A burst reads only the seed, the trigger count, and the phase.

// Burst lines share one column: tags are padded to the field indent.
const (
	promptPrefix   = "nightshift> "
	okPrefix       = "[ok]   "
	warnPrefix     = "[warn] "
	progressPrefix = "[>>]   "
	fieldIndent    = "       "
	progressCells  = 20
)

type site struct {
	tag  string
	host string
	addr string
	os   string
	role string
	find string
	seal string
}

var sites = []site{
	{"web-01", "web-01.ember.invalid", "192.0.2.44", "Debian 12", "edge proxy", "FND-2041", "SIM-7F"},
	{"db-02", "db-02.ember.invalid", "192.0.2.45", "Debian 12", "postgres primary", "FND-1880", "SIM-2C"},
	{"vpn-gw", "vpn-gw.lantern.invalid", "198.51.100.7", "OpenBSD 7.5", "vpn gateway", "FND-2204", "SIM-9A"},
	{"build-07", "build-07.lantern.invalid", "198.51.100.23", "Ubuntu 24.04", "ci runner", "FND-1766", "SIM-4D"},
	{"mail-01", "mail-01.dock.invalid", "203.0.113.10", "Rocky 9", "mail relay", "FND-2310", "SIM-1B"},
	{"jump-01", "jump-01.loft.invalid", "203.0.113.4", "Debian 12", "bastion", "FND-1599", "SIM-6E"},
	{"files-03", "files-03.annex.invalid", "192.0.2.80", "FreeBSD 14", "file server", "FND-2402", "SIM-8C"},
	{"dc-01", "dc-01.index.invalid", "198.51.100.90", "Server 2022", "directory", "FND-1420", "SIM-3A"},
	{"k8s-n4", "k8s-node-4.yard.invalid", "203.0.113.55", "Talos 1.7", "cluster node", "FND-2677", "SIM-5F"},
	{"hpot-2", "honeypot-2.gallery.invalid", "192.0.2.18", "Debian 12", "decoy", "FND-1904", "SIM-7A"},
}

// hostile addresses are the far end of every fictional incident.
var hostile = [...]string{"198.51.100.201", "203.0.113.66", "198.51.100.143", "203.0.113.217"}

type rng struct{ state uint64 }

func (r *rng) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	return splitmix(r.state)
}

func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }

func (r *rng) between(low, high int) int { return low + r.intn(high-low+1) }

func pick[T any](r *rng, items []T) T { return items[r.intn(len(items))] }

// burst carries one burst's seeded choices.
type burst struct {
	r      *rng
	target site
	peer   site
	remote string
	clock  int // milliseconds since midnight, always in the night shift
}

func (b *burst) tick() {
	b.clock = (b.clock + b.r.between(40, 2400)) % 86400000
}

func (b *burst) stamp() string {
	b.tick()
	c := b.clock
	return fmt.Sprintf("%02d:%02d:%02d.%03d", c/3600000, c/60000%60, c/1000%60, c%1000)
}

func (b *burst) shortStamp() string {
	return b.stamp()[:8]
}

func (b *burst) docAddr() string {
	prefix := pick(b.r, []string{"192.0.2.", "198.51.100.", "203.0.113."})
	return fmt.Sprintf("%s%d", prefix, b.r.between(1, 254))
}

func (b *burst) hexDigits(n int) string {
	const digits = "0123456789abcdef"
	out := make([]byte, n)
	for i := range out {
		out[i] = digits[b.r.intn(16)]
	}
	return string(out)
}

func prompt(cmd string) string { return promptPrefix + cmd }

func detail(format string, args ...any) string {
	return strings.TrimRight(fieldIndent+fmt.Sprintf(format, args...), " ")
}

func field(label, value string) string { return detail("%-8s %s", label, value) }

func okLine(format string, args ...any) string {
	return okPrefix + fmt.Sprintf(format, args...)
}

func warnLine(format string, args ...any) string {
	return warnPrefix + fmt.Sprintf(format, args...)
}

func meterLine(tag string, filled int) string {
	return detail("%-8s [%s%s]", tag, strings.Repeat("#", filled), strings.Repeat("-", 8-filled))
}

// progressLine is a finished loader. The writer may animate it filling up.
func progressLine(label string) string {
	return progressFrame(label, progressCells)
}

func progressFrame(label string, filled int) string {
	bars := strings.Repeat("#", filled) + strings.Repeat("-", progressCells-filled)
	return fmt.Sprintf(progressPrefix+"%-13s [%s] %3d%%", label, bars, filled*100/progressCells)
}

// hexRow is one dump row of eight bytes with an ASCII gutter.
func hexRow(offset int, data []byte) string {
	hex := make([]string, len(data))
	gutter := make([]byte, len(data))
	for i, b := range data {
		hex[i] = fmt.Sprintf("%02x", b)
		switch {
		case b >= '0' && b <= '9', b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b == '=', b == '-', b == '_':
			gutter[i] = b
		default:
			gutter[i] = '.'
		}
	}
	return detail("0x%04x  %s  |%s|", offset, strings.Join(hex, " "), gutter)
}

type template func(b *burst) []string

// phasePools keep each phase's bursts in character with its objective.
var phasePools = [phaseCount][]template{
	PhaseBoot:        {keyAgent, integrityCheck, hostStatus, certProbe, processList, clockSkew, signatureCheck, diskUsage},
	PhaseSignal:      {portScan, packetCapture, dnsLookup, traceRoute, socketList, flowTop, idsAlert},
	PhaseCorrelation: {authLog, memoryDump, proxyLog, yaraScan, hostTimeline, idsAlert, packetCapture},
	PhaseContainment: {firewallDeny, edrIsolate, evidenceSnapshot, secretRotate, stopProcess, processList, integrityCheck},
	PhaseReport:      {sealReport, evidenceArchive, iocExport, ticketUpdate, integrityCheck, certProbe},
}

// templateOrder shuffles a phase pool once per seed. Walking it by trigger
// count means two bursts in a row never share a template.
func templateOrder(seed uint64, phase Phase) []int {
	pool := phasePools[phase]
	order := make([]int, len(pool))
	for i := range order {
		order[i] = i
	}
	r := &rng{state: seed ^ uint64(phase+1)*0xd1b54a32d192ed03}
	for i := len(order) - 1; i > 0; i-- {
		j := r.intn(i + 1)
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// burstLines is the burst for this seed, zero-based trigger count, and phase.
func burstLines(seed, trigger uint64, phase Phase) []string {
	if phase < 0 || int(phase) >= phaseCount {
		phase = PhaseBoot
	}
	r := &rng{state: splitmix(seed^(trigger+1)*0xa0761d6478bd642f) ^ uint64(phase)*0xe7037ed1a0b428db}
	b := &burst{r: r, target: pick(r, sites), remote: pick(r, hostile[:])}
	b.peer = sites[(r.intn(len(sites)-1)+1+indexOf(b.target))%len(sites)]
	// The shift runs 01:00 to 04:59; each trigger moves the clock on.
	b.clock = int(splitmix(seed)%4)*3600000 + 3600000 + int(splitmix(seed^1)%1800000) + int(trigger)*47000
	order := templateOrder(seed, phase)
	pool := phasePools[phase]
	return pool[order[int(trigger)%len(order)]](b)
}

func indexOf(s site) int {
	for i, candidate := range sites {
		if candidate.host == s.host {
			return i
		}
	}
	return 0
}

func portScan(b *burst) []string {
	services := []struct{ port, name, version string }{
		{"22/tcp", "ssh", "OpenSSH_9.6p1"},
		{"53/udp", "domain", "-"},
		{"80/tcp", "http", "nginx 1.25.3"},
		{"443/tcp", "https", "nginx 1.25.3"},
		{"3306/tcp", "mysql", "8.0.36"},
		{"5432/tcp", "postgresql", "16.2"},
		{"6443/tcp", "k8s-api", "-"},
		{"8080/tcp", "http-proxy", "-"},
		{"9100/tcp", "metrics", "-"},
		{"3389/tcp", "ms-wbt", "-"},
	}
	count := b.r.between(3, 5)
	start := b.r.intn(len(services))
	rows := make([]int, count)
	for i := range rows {
		rows[i] = (start + i*3) % len(services)
	}
	sort.Slice(rows, func(i, j int) bool {
		var left, right int
		fmt.Sscanf(services[rows[i]].port, "%d", &left)
		fmt.Sscanf(services[rows[j]].port, "%d", &right)
		return left < right
	})
	lines := []string{
		prompt("scan -sS -sV -T4 --top-ports 1000 " + b.target.host),
		progressLine("syn sweep"),
		detail("%-9s %-9s %-11s %s", "PORT", "STATE", "SERVICE", "VERSION"),
	}
	open := 0
	for _, row := range rows {
		service := services[row]
		state := pick(b.r, []string{"open", "open", "open", "filtered", "closed"})
		if state == "open" {
			open++
		}
		lines = append(lines, detail("%-9s %-9s %-11s %s", service.port, state, service.name, service.version))
	}
	if b.r.intn(4) == 0 {
		lines = append(lines, detail("Warning: retransmission cap hit (%d)", b.r.between(2, 10)))
	}
	return append(lines, okLine("%s (%s)  %d open  %d.%02ds", b.target.host, b.target.addr, open, b.r.between(1, 40), b.r.intn(100)))
}

func packetCapture(b *burst) []string {
	count := b.r.between(4, 6)
	lines := []string{
		prompt(fmt.Sprintf("capture -i eth0 -c %d host %s", count, b.remote)),
		progressLine("pcap attach"),
	}
	flags := []string{"[S]", "[S.]", "[.]", "[P.]", "[P.]", "[F.]"}
	port := b.r.between(40000, 60999)
	for i := 0; i < count; i++ {
		src, dst := fmt.Sprintf("%s:%d", b.target.addr, port), b.remote+":443"
		if i%2 == 1 {
			src, dst = dst, src
		}
		size := 0
		if strings.HasPrefix(flags[i], "[P") {
			size = b.r.between(90, 1448)
		}
		lines = append(lines, detail("%s  %s > %s  %-4s %d", b.shortStamp(), src, dst, flags[i], size))
	}
	return append(lines, warnLine("beacon to %s  interval 60.%ds  jitter %d%%", b.remote, b.r.intn(10), b.r.between(2, 9)))
}

func authLog(b *burst) []string {
	count := b.r.between(4, 6)
	lines := []string{prompt("tail -n 200 /var/log/auth.log | grep sshd")}
	pid := b.r.between(3000, 8999)
	fails := 0
	for i := 0; i < count-1; i++ {
		user := pick(b.r, []string{"root", "ops", "git", "pi", "test"})
		lines = append(lines, detail("%s sshd[%d]: Failed password for %s from %s", b.shortStamp(), pid, user, b.remote))
		fails++
		if b.r.intn(3) == 0 {
			pid++
		}
	}
	lines = append(lines, detail("%s sshd[%d]: Accepted publickey for ir from %s", b.shortStamp(), pid+1, b.peer.addr))
	return append(lines, warnLine("%s  %d failures in %ds  threshold 5", b.remote, fails+b.r.between(4, 12), b.r.between(20, 90)))
}

func dnsLookup(b *burst) []string {
	name := b.target.host + "."
	lines := []string{prompt("dig +noall +answer " + b.target.host + " A AAAA MX TXT")}
	if b.r.intn(4) == 0 {
		lines = append(lines, detail(";; communications error to 192.0.2.53#53: timed out"))
	}
	lines = append(lines,
		detail("%-27s %4d IN %-4s %s", name, 300, "A", b.target.addr),
		detail("%-27s %4d IN %-4s 2001:db8::%x", name, 300, "AAAA", b.r.between(16, 255)),
	)
	if b.r.intn(2) == 0 {
		lines = append(lines, detail("%-27s %4d IN %-4s 10 %s.", name, 3600, "MX", sites[4].host))
	}
	lines = append(lines, detail("%-27s %4d IN %-4s \"v=spf1 -all\"", name, 3600, "TXT"))
	answers := 0
	for _, line := range lines[1:] {
		if strings.Contains(line, " IN ") {
			answers++
		}
	}
	return append(lines, okLine("%d answers  %d msec  server 192.0.2.53", answers, b.r.between(4, 61)))
}

func traceRoute(b *burst) []string {
	hops := b.r.between(4, 6)
	lines := []string{prompt("trace -n -q 1 " + b.remote)}
	latency := 0.3
	for hop := 1; hop <= hops; hop++ {
		latency += float64(b.r.between(200, 9000)) / 1000
		switch {
		case hop == hops:
			lines = append(lines, detail("%2d  %-15s %7.3f ms", hop, b.remote, latency))
		case hop > 1 && b.r.intn(4) == 0:
			lines = append(lines, detail("%2d  * * *", hop))
		default:
			lines = append(lines, detail("%2d  %-15s %7.3f ms", hop, b.docAddr(), latency))
		}
	}
	return append(lines, warnLine("%s reached in %d hops  ttl %d  asn 64%03d", b.remote, hops, b.r.between(44, 58), b.r.intn(1000)))
}

func processList(b *burst) []string {
	procs := []struct{ user, comm string }{
		{"root", "/usr/sbin/sshd"},
		{"www", "nginx: worker"},
		{"postgres", "postgres: wal"},
		{"root", "containerd"},
		{"ops", "gunicorn: worker"},
		{"root", "kworker/2:1"},
		{"node", "node /srv/api"},
	}
	count := b.r.between(3, 4)
	lines := []string{
		prompt("ps -eo pid,user,pcpu,comm --sort=-pcpu | head -n " + fmt.Sprint(count+1)),
		detail("%-6s %-9s %5s  %s", "PID", "USER", "%CPU", "COMMAND"),
	}
	start := b.r.intn(len(procs))
	cpu := float64(b.r.between(300, 900)) / 10
	for i := 0; i < count-1; i++ {
		p := procs[(start+i)%len(procs)]
		lines = append(lines, detail("%-6d %-9s %5.1f  %s", b.r.between(300, 9999), p.user, cpu, p.comm))
		cpu /= 1.7
	}
	rogue := b.r.between(4000, 9999)
	path := pick(b.r, []string{"/tmp/.cache/syncd", "/dev/shm/.x/kdump", "/var/tmp/.font-unix/agent"})
	lines = append(lines, detail("%-6d %-9s %5.1f  %s", rogue, "www", cpu, path))
	return append(lines, warnLine("pid %d  unsigned binary  parent 1  %s", rogue, b.target.host))
}

func firewallDeny(b *burst) []string {
	rule := b.r.between(10, 99)
	nodes := b.r.between(2, 6)
	return []string{
		prompt(fmt.Sprintf("fw deny from %s --comment %s", b.remote, b.target.find)),
		progressLine("rule push"),
		detail("chain INPUT  rule %d  DROP  src %s/32", rule, b.remote),
		detail("chain OUTPUT rule %d  DROP  dst %s/32", rule+1, b.remote),
		detail("pushed to %d nodes in %d ms  (%s, %s)", nodes, b.r.between(80, 900), b.target.tag, b.peer.tag),
		okLine("%s blocked  rules %d-%d  %s", b.remote, rule, rule+1, b.target.seal),
		meterLine(b.target.tag, b.r.between(5, 8)),
	}
}

func integrityCheck(b *burst) []string {
	files := []string{
		"/usr/sbin/sshd", "/etc/ssh/sshd_config", "/usr/lib/libaudit.so.1",
		"/etc/pam.d/common-auth", "/usr/bin/sudo", "/etc/cron.d/backup",
	}
	count := b.r.between(3, 5)
	start := b.r.intn(len(files))
	bad := -1
	if b.r.intn(3) > 0 {
		bad = b.r.intn(count)
	}
	lines := []string{prompt("sha256sum -c /srv/baseline/" + b.target.tag + ".sha256")}
	for i := 0; i < count; i++ {
		verdict := "OK"
		if i == bad {
			verdict = "FAILED"
		}
		lines = append(lines, detail("%s: %s", files[(start+i)%len(files)], verdict))
	}
	if bad < 0 {
		return append(lines, okLine("%d files match the baseline  %s", count, b.target.host))
	}
	return append(lines, warnLine("WARNING: 1 computed checksum did NOT match"))
}

func certProbe(b *burst) []string {
	// Ten pairs, cut short as a console would; eight would read as IPv6.
	fingerprint := make([]string, 10)
	for i := range fingerprint {
		fingerprint[i] = strings.ToUpper(b.hexDigits(2))
	}
	return []string{
		prompt("tls probe " + b.target.host + ":443"),
		progressLine("tls handshake"),
		field("protocol", "TLSv1.3"),
		field("cipher", "TLS_AES_256_GCM_SHA384"),
		field("subject", "CN="+b.target.host),
		field("issuer", "CN=Night Slate Intermediate R3"),
		field("expires", fmt.Sprintf("2027-%02d-%02d  (%d days)", b.r.between(1, 12), b.r.between(1, 28), b.r.between(40, 390))),
		field("sha256", strings.Join(fingerprint, ":")+":..."),
		okLine("chain verified  depth 2  ocsp good"),
	}
}

func memoryDump(b *burst) []string {
	pid := b.r.between(3000, 9999)
	base := b.r.between(0x1000, 0xefff) &^ 0xf
	payload := []byte(fmt.Sprintf("beacon-v2 sleep=60 jit=%d id=%s", b.r.between(2, 9), b.hexDigits(6)))
	noise := make([]byte, 8)
	for i := range noise {
		noise[i] = byte(b.r.next())
	}
	return []string{
		prompt(fmt.Sprintf("dump --pid %d --region heap --grep beacon", pid)),
		progressLine("memory map"),
		hexRow(base, noise),
		hexRow(base+8, payload[:8]),
		hexRow(base+16, payload[8:16]),
		hexRow(base+24, payload[16:24]),
		warnLine("pattern 'beacon' at 0x%04x  pid %d  %s", base+8, pid, b.target.host),
	}
}

func hostStatus(b *burst) []string {
	state := pick(b.r, []string{"up", "degraded", "isolated", "quarantined", "listening"})
	lines := []string{
		prompt("status --verbose " + b.target.host),
		okLine("%s  %s", b.target.host, state),
		field("addr", b.target.addr),
		field("os", b.target.os),
		field("role", b.target.role),
	}
	if b.r.intn(2) == 0 {
		lines = append(lines, field("uptime", fmt.Sprintf("%dd %02d:%02d", b.r.between(1, 90), b.r.intn(24), b.r.intn(60))))
	}
	return append(lines, meterLine(b.target.tag, b.r.between(1, 8)))
}

func keyAgent(b *burst) []string {
	keys := []string{
		detail("256 SHA256:%s ops@nightshift (ED25519)", b.hexDigits(24)),
		detail("256 SHA256:%s ir@nightshift (ED25519)", b.hexDigits(24)),
		detail("4096 SHA256:%s backup@nightshift (RSA)", b.hexDigits(22)),
	}
	count := b.r.between(2, 3)
	lines := append([]string{prompt("ssh-add -l")}, keys[:count]...)
	return append(lines, okLine("%d identities loaded  agent pid %d", count, b.r.between(300, 9999)))
}

func sealReport(b *burst) []string {
	report := fmt.Sprintf("AAR-%02d", b.r.between(10, 99))
	start := b.clock
	b.clock += b.r.between(600000, 1800000)
	window := func(ms int) string {
		return fmt.Sprintf("%02d:%02d:%02d", ms/3600000%24, ms/60000%60, ms/1000%60)
	}
	return []string{
		prompt("report --seal " + report),
		field("findings", fmt.Sprintf("%d  %s  %s", b.r.between(2, 5), b.target.find, b.peer.find)),
		field("hosts", fmt.Sprintf("%d touched  0 persisted", b.r.between(2, 7))),
		field("window", window(start)+" -> "+window(b.clock)),
		field("digest", b.hexDigits(16)+"..."),
		progressLine("report seal"),
		okLine("%s sealed  %s  handed to day shift", report, b.target.seal),
	}
}

func clockSkew(b *burst) []string {
	offset := b.r.between(8, 900)
	sign := pick(b.r, []string{"+", "-"})
	lines := []string{
		prompt("chronyc tracking"),
		field("ref", "192.0.2.123 (ntp1.index.invalid)"),
		field("stratum", fmt.Sprint(b.r.between(2, 3))),
		field("offset", fmt.Sprintf("%s0.000%03d s", sign, offset)),
		field("freq", fmt.Sprintf("%d.%03d ppm slow", b.r.between(1, 19), b.r.intn(1000))),
		field("leap", "Normal"),
	}
	if b.r.intn(5) == 0 {
		return append(lines, warnLine("skew %s1.%03ds on %s  exceeds 500ms", sign, b.r.intn(1000), b.peer.tag))
	}
	return append(lines, okLine("clock skew %dus  within tolerance", offset))
}

func signatureCheck(b *burst) []string {
	key := strings.ToUpper(b.hexDigits(16))
	return []string{
		prompt("gpg --verify /opt/kits/ir-tools.tar.sig"),
		detail("gpg: Signature made Tue 22 Sep 2026 00:%02d:%02d UTC", b.r.intn(60), b.r.intn(60)),
		detail("gpg:                using EDDSA key %s", key),
		detail("gpg: Good signature from \"IR Tooling <ir@nightshift>\""),
		okLine("toolkit signature valid  %d files  key %s", b.r.between(40, 400), key[12:]),
	}
}

func diskUsage(b *burst) []string {
	mounts := []struct{ dev, mount string }{
		{"/dev/nvme0n1p2", "/"}, {"/dev/nvme0n1p3", "/var"},
		{"/dev/nvme1n1p1", "/srv"}, {"tmpfs", "/tmp"},
	}
	lines := []string{
		prompt("df -h / /var /srv /tmp"),
		detail("%-15s %5s %5s %5s %4s  %s", "Filesystem", "Size", "Used", "Avail", "Use%", "Mounted"),
	}
	worst := 0
	for _, m := range mounts {
		size := pick(b.r, []int{16, 40, 120, 480})
		use := b.r.between(8, 97)
		worst = max(worst, use)
		used := size * use / 100
		lines = append(lines, detail("%-15s %4dG %4dG %4dG %3d%%  %s", m.dev, size, used, size-used, use, m.mount))
	}
	if worst >= 90 {
		return append(lines, warnLine("a volume is at %d%%  log rotation queued", worst))
	}
	return append(lines, okLine("all volumes under 90%%  %s", b.target.host))
}

func socketList(b *burst) []string {
	count := b.r.between(3, 4)
	lines := []string{
		prompt("ss -tnp state established"),
		detail("%-20s %-21s %s", "Local", "Peer", "Process"),
	}
	for i := 0; i < count-1; i++ {
		lines = append(lines, detail("%-20s %-21s %s",
			fmt.Sprintf("%s:%d", b.target.addr, pick(b.r, []int{22, 443, 5432})),
			fmt.Sprintf("%s:%d", b.docAddr(), b.r.between(40000, 60999)),
			pick(b.r, []string{"sshd/412", "nginx/988", "postgres/77"})))
	}
	pid := b.r.between(3000, 8999)
	lines = append(lines, detail("%-20s %-21s %s",
		fmt.Sprintf("%s:%d", b.target.addr, b.r.between(40000, 60999)), b.remote+":443", fmt.Sprintf("syncd/%d", pid)))
	return append(lines, warnLine("%s:443  held %dm%02ds  by syncd/%d", b.remote, b.r.between(3, 59), b.r.intn(60), pid))
}

func flowTop(b *burst) []string {
	lines := []string{
		prompt("flow top --by bytes --last 15m"),
		detail("%-15s %-15s %5s %7s %5s", "SRC", "DST", "PORT", "BYTES", "FLOWS"),
		detail("%-15s %-15s %5d %6dM %5d", b.target.addr, b.remote, 443, b.r.between(180, 900), b.r.between(12, 16)),
	}
	for i := 0; i < b.r.between(2, 3); i++ {
		lines = append(lines, detail("%-15s %-15s %5d %6dM %5d", b.docAddr(), b.docAddr(),
			pick(b.r, []int{53, 443, 5432, 8080}), b.r.between(4, 170), b.r.between(40, 900)))
	}
	return append(lines, warnLine("egress to %s is %dx its 7-day baseline", b.remote, b.r.between(6, 40)))
}

func idsAlert(b *burst) []string {
	names := []string{"TLS beacon to rare host", "DNS TXT query burst", "SSH brute force", "JA3 mismatch on 443", "Long-lived idle TLS"}
	count := b.r.between(2, 3)
	lines := []string{prompt("ids tail --severity 2 -n " + fmt.Sprint(count))}
	for i := 0; i < count; i++ {
		lines = append(lines,
			detail("%s [1:%d:%d] NS %s", b.shortStamp(), b.r.between(9000100, 9000999), b.r.between(1, 4), pick(b.r, names)),
			detail("         %s:%d -> %s:443  {TCP}", b.target.addr, b.r.between(40000, 60999), b.remote))
	}
	return append(lines, warnLine("%d alerts in 10m  top talker %s", count+b.r.between(0, 9), b.target.host))
}

func proxyLog(b *burst) []string {
	count := b.r.between(3, 5)
	lines := []string{prompt(fmt.Sprintf("grep %s /var/log/nginx/access.log | tail -n %d", b.remote, count))}
	for i := 0; i < count; i++ {
		request := pick(b.r, []string{"POST /api/v2/checkin", "POST /api/v2/checkin", "GET /api/v2/tasks", "GET /static/app.js"})
		lines = append(lines, detail("%s - - [%s] \"%s\" %d %d", b.remote, b.shortStamp(), request, pick(b.r, []int{200, 200, 204, 404}), b.r.between(90, 4200)))
	}
	return append(lines, warnLine("%d requests  mean interval 60.%ds  one user agent", count+b.r.between(20, 300), b.r.intn(10)))
}

func yaraScan(b *burst) []string {
	path := pick(b.r, []string{"/tmp/.cache/syncd", "/dev/shm/.x/kdump", "/var/tmp/.font-unix/agent"})
	lines := []string{
		prompt("yara -r /opt/rules/beacon.yar /tmp /var/tmp /dev/shm"),
		progressLine("yara scan"),
		detail("BeaconV2_Loader %s", path),
	}
	if b.r.intn(2) == 0 {
		lines = append(lines, detail("BeaconV2_Config %s", path))
	}
	lines = append(lines, detail("scanned %d files in %d.%ds", b.r.between(900, 40000), b.r.between(1, 30), b.r.intn(10)))
	return append(lines, warnLine("%d rule hits on %s  sha256 %s...", len(lines)-3, b.target.tag, b.hexDigits(12)))
}

func hostTimeline(b *burst) []string {
	events := []struct{ kind, text string }{
		{"session", "ssh ops from " + b.peer.addr},
		{"exec", "/tmp/.cache/syncd --quiet"},
		{"net", "connect " + b.remote + ":443"},
		{"file", "write /etc/cron.d/backup"},
		{"priv", "sudo by www  denied"},
		{"net", "dns TXT cdn-sync.gallery.invalid"},
	}
	count := b.r.between(4, 5)
	start := b.r.intn(len(events))
	lines := []string{prompt("timeline --host " + b.target.host + " --window 30m")}
	first := ""
	for i := 0; i < count; i++ {
		event := events[(start+i)%len(events)]
		stamp := b.shortStamp()
		if first == "" {
			first = stamp
		}
		lines = append(lines, detail("%s  %-8s %s", stamp, event.kind, event.text))
	}
	return append(lines, warnLine("first seen %s  dwell %dm  %s", first, b.r.between(9, 140), b.target.find))
}

func edrIsolate(b *burst) []string {
	lines := []string{
		prompt(fmt.Sprintf("edr isolate %s --allow %s", b.target.host, sites[5].tag)),
		progressLine("policy push"),
	}
	if b.r.intn(3) == 0 {
		lines = append(lines, detail("retry 1/3: agent busy, backing off %dms", b.r.between(200, 900)))
	}
	return append(lines,
		field("network", "isolated  allow "+sites[5].tag+", 192.0.2.53"),
		field("sessions", fmt.Sprintf("%d dropped", b.r.between(1, 9))),
		okLine("%s isolated  INC-%d", b.target.host, b.r.between(4000, 4999)),
	)
}

func evidenceSnapshot(b *burst) []string {
	dir := "/evidence/" + b.target.tag
	return []string{
		prompt("snapshot --memory --disk " + b.target.host),
		progressLine("memory image"),
		progressLine("disk image"),
		detail("%s/mem.lime   %2d.%d GiB", dir, b.r.between(4, 64), b.r.intn(10)),
		detail("%s/disk.raw   %2d.%d GiB", dir, b.r.between(20, 99), b.r.intn(10)),
		okLine("evidence sealed  custody %s  %s", b.target.seal, b.target.find),
	}
}

func secretRotate(b *burst) []string {
	scopes := []string{"db", "api", "ssh", "backup"}
	count := b.r.between(2, 3)
	start := b.r.intn(len(scopes))
	lines := []string{prompt("vault rotate --scope svc/" + b.target.tag)}
	for i := 0; i < count; i++ {
		lines = append(lines, detail("rotated  svc/%s/%-7s lease %dh  old revoked", b.target.tag, scopes[(start+i)%len(scopes)], pick(b.r, []int{1, 8, 24, 72})))
	}
	return append(lines, okLine("%d secrets rotated  %d sessions invalidated", count, b.r.between(0, 14)))
}

func stopProcess(b *burst) []string {
	pid := b.r.between(3000, 8999)
	return []string{
		prompt(fmt.Sprintf("kill -STOP %d && gcore -o /evidence/%s %d", pid, b.target.tag, pid)),
		detail("Saved corefile /evidence/%s.%d", b.target.tag, pid),
		detail("%d MiB written in %d.%ds", b.r.between(12, 480), b.r.between(0, 6), b.r.intn(10)),
		prompt(fmt.Sprintf("kill -KILL %d", pid)),
		okLine("pid %d stopped  imaged  %s", pid, b.target.host),
	}
}

func evidenceArchive(b *burst) []string {
	report := fmt.Sprintf("AAR-%02d", b.r.between(10, 99))
	raw := b.r.between(8, 90)
	return []string{
		prompt(fmt.Sprintf("tar czf /evidence/%s.tgz /evidence/%s", report, b.target.tag)),
		progressLine("compress"),
		detail("%d files  %d.%d GiB -> %d.%d GiB", b.r.between(40, 900), raw, b.r.intn(10), raw/3, b.r.intn(10)),
		field("sha256", b.hexDigits(40)+"..."),
		okLine("archive written  %s  %s", report, b.target.seal),
	}
}

func iocExport(b *burst) []string {
	return []string{
		prompt("ioc export --format csv --since 01:00"),
		detail("%-8s %-28s %s", "TYPE", "VALUE", "SEEN"),
		detail("%-8s %-28s %s", "ipv4", b.remote, b.shortStamp()),
		detail("%-8s %-28s %s", "domain", "cdn-sync.gallery.invalid", b.shortStamp()),
		detail("%-8s %-28s %s", "sha256", b.hexDigits(24)+"...", b.shortStamp()),
		detail("%-8s %-28s %s", "path", "/tmp/.cache/syncd", b.shortStamp()),
		okLine("%d indicators exported  shared with day shift", 4+b.r.between(0, 12)),
	}
}

func ticketUpdate(b *burst) []string {
	ticket := fmt.Sprintf("INC-%d", b.r.between(4000, 4999))
	return []string{
		prompt("ticket update " + ticket + " --status contained"),
		field("status", "investigating -> contained"),
		field("owner", "night-ir"),
		field("assets", b.target.tag+", "+b.peer.tag),
		field("sla", fmt.Sprintf("met  %dm to spare", b.r.between(4, 90))),
		okLine("%s updated  %d watchers notified", ticket, b.r.between(2, 11)),
	}
}
