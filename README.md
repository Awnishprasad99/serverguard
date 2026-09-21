# serverguard

# 🛡️ ServerGuard

> A terminal-based Linux Server Control Center built with Go.

ServerGuard is a lightweight terminal application designed to bring essential Linux server monitoring and administration information into one interface.

It is useful for:

- Linux beginners
- System Administrators
- DevOps Engineers
- Cloud Engineers
- Linux enthusiasts

---

## ✨ Features

### 📊 System Monitoring

- Hostname information
- Operating system details
- Kernel and architecture information
- System uptime
- CPU usage and system load
- RAM and memory usage
- Disk usage
- Network upload and download information

### ⚙️ Process Management

- View running processes
- Search for processes
- Navigate through process entries
- Review CPU and memory usage
- Select one or multiple processes
- Process termination confirmation
- Process risk warnings
- Critical-process protection
- Parent/child process information where supported

> ⚠️ Always verify a process before terminating it. Stopping critical system processes may affect server stability.

### 🌐 Network and Firewall Information

- Network information
- Listening ports
- Protocol details
- UFW firewall rules and status, where available

### 🐳 Docker Monitoring

- Total container count
- Running container count
- Docker image count
- Container names and status
- Container port mappings

Example:

```text
0.0.0.0:8000->9000/tcp
```

This means that port `8000` on the host is mapped to port `9000` inside the container.

### 🔧 Service Information

ServerGuard can display information about important services, depending on the services installed on your Linux system:

- SSH / sshd
- Docker
- Nginx
- containerd
- UFW

---

## 🖥️ Requirements

Before installing ServerGuard, make sure your system has:

- A Linux operating system
- Go installed
- Git
- Docker (optional, for Docker monitoring)
- UFW (optional, for firewall information)
- Required permissions to read system and service information

---

## 📥 Installation

### Method 1: Clone the Repository

Open your Linux terminal and run:

```bash
git clone https://github.com/Awnishprasad99/serverguard.git
```

Enter the project directory:

```bash
cd serverguard
```

### Method 2: Download ZIP

1. Open the GitHub repository:

   https://github.com/Awnishprasad99/serverguard

2. Click the green **Code** button.
3. Select **Download ZIP**.
4. Extract the ZIP file.
5. Open the extracted project directory.

---

## 🔨 Install Dependencies

From the project directory, run:

```bash
go mod download
```

Update and organize Go dependencies:

```bash
go mod tidy
```

> If the repository already contains a `go.mod` file, use the existing module configuration.

---

## 🏗️ Build ServerGuard

Build the application using:

```bash
go build -o serverguard
```

Check the generated binary:

```bash
ls -lh serverguard
```

---

## ▶️ Run ServerGuard

Give execution permission:

```bash
chmod +x serverguard
```

Start ServerGuard:

```bash
./serverguard
```

> Use `sudo` only when a specific feature requires elevated permissions.

---

## ⌨️ Keyboard Controls

The available controls may vary depending on the screen and application version.

| Key | Action |
|---|---|
| `P` | Open process manager |
| `D` | Open Docker information |
| `N` | Open network and firewall information |
| `R` | Refresh information |
| `/` | Search processes |
| `↑` / `↓` | Navigate |
| `j` / `k` | Navigate in supported views |
| `Space` | Select a process |
| `K` | Open process termination confirmation |
| `Backspace` | Delete search text |
| `Esc` | Return to the previous screen |
| `Q` | Quit the application |

---

## ⚙️ Process Management Workflow

1. Open the process manager.
2. Search for a process if required.
3. Select a process using `Space`.
4. Press capital `K`.
5. Review the selected process and warning.
6. Confirm the action only after verifying the process.

### Security Warning

Do not terminate critical processes such as:

- `systemd`
- `sshd`
- Essential system services
- Active production workloads

Always test potentially destructive operations on a non-production server first.

---

## 🐳 Docker Commands

Check the Docker version:

```bash
docker --version
```

Show running containers:

```bash
docker ps
```

Show all containers:

```bash
docker ps -a
```

Show Docker images:

```bash
docker images
```

Show container names, status, images, and ports:

```bash
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Image}}\t{{.Ports}}"
```

Check Docker service status:

```bash
sudo systemctl status docker
```

---

## 🌐 Network and Firewall Commands

Show listening ports and processes:

```bash
sudo ss -tulnp
```

Show network interfaces:

```bash
ip addr show
```

Check network connectivity:

```bash
curl google.com
```

Check UFW firewall status:

```bash
sudo ufw status verbose
```

Show numbered UFW rules:

```bash
sudo ufw status numbered
```

### Network Terms

| Term | Meaning |
|---|---|
| TCP | Connection-oriented network protocol |
| UDP | Connectionless network protocol |
| LISTEN | Service waiting for incoming connections |
| `0.0.0.0` | Listening on all IPv4 interfaces |
| `[::]` | Listening on all IPv6 interfaces |

> A listening port is not automatically reachable from the internet. Firewall rules, cloud security groups, routing, and provider-level restrictions may also apply.

---

## 🔧 Service Management Commands

Check SSH:

```bash
systemctl status ssh
```

Check Docker:

```bash
systemctl status docker
```

Check Nginx:

```bash
systemctl status nginx
```

Check containerd:

```bash
systemctl status containerd
```

Check UFW:

```bash
sudo ufw status verbose
```

> Service names can differ between Linux distributions.

---

## 🔐 Security Recommendations

- Run ServerGuard with the minimum permissions required.
- Verify a process before terminating it.
- Do not expose management ports unnecessarily.
- Keep your Linux system updated.
- Keep Docker packages updated.
- Allow only required firewall traffic.
- Avoid using `sudo` unnecessarily.
- Test destructive operations on a non-production server.
- Take backups before making major server changes.

---

## 🧪 Troubleshooting

### Go Command Not Found

Check whether Go is installed:

```bash
go version
```

If Go is not installed, install it using the supported installation method for your Linux distribution.

---

### Build Errors

Run the following commands:

```bash
go mod tidy
```

```bash
go mod download
```

```bash
go build -o serverguard
```

---

### Docker Information Is Not Available

Check Docker installation:

```bash
docker --version
```

Check the Docker service:

```bash
sudo systemctl status docker
```

Test Docker access:

```bash
docker ps
```

---

### UFW Information Is Not Available

Check UFW status:

```bash
sudo ufw status verbose
```

If UFW is not installed, install it using your Linux distribution's package manager.

---

### Service Information Is Not Available

Check the service manually:

```bash
systemctl status ssh
```

```bash
systemctl status docker
```

```bash
systemctl status nginx
```

```bash
systemctl status containerd
```

---

## 🚀 Future Improvements

Planned or possible improvements include:

- Detailed Docker container inspection
- Container logs
- Container restart controls
- Service start, stop, and restart options
- Configurable refresh intervals
- Monitoring data export
- Improved error handling
- Better permission messages
- Support for additional Linux distributions
- Prebuilt binaries and installation packages

---

## 🤝 Contributing

Contributions, suggestions, bug reports, and improvements are welcome.

### Clone the Repository

```bash
git clone https://github.com/Awnishprasad99/serverguard.git
```

```bash
cd serverguard
```

### Create a Feature Branch

```bash
git checkout -b feature/your-feature
```

Make your changes, test them, and create a pull request.

Please explain your changes clearly in the pull request description.

---

## 🔗 Project Links

- **GitHub Repository:**  
  https://github.com/Awnishprasad99/serverguard

- **Issues:**  
  https://github.com/Awnishprasad99/serverguard/issues

- **Releases:**  
  https://github.com/Awnishprasad99/serverguard/releases

---

## 👨‍💻 Author

**Awnish**

Built while learning and working with:

- Linux
- Cloud Computing
- Go (Golang)
- Docker
- DevOps
- System Administration

---

## ⭐ Support the Project

If you find ServerGuard useful:

- ⭐ Star the repository
- 🐛 Report bugs
- 💡 Suggest improvements
- 🤝 Contribute to the project
- 📢 Share it with the Linux and DevOps community

---

> 🚀 Small tools can make a big difference.
>
> **Keep Learning. Keep Building. Keep Growing.**
