package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"text/template"
	"strings"
)

type Config struct {
	SupervisorSockPath string
	SupervisorLogPath  string
	SupervisorDir      string
	AirflowHome        string
	AirflowEnv         string
	AirflowUser        string
	AirflowPort        string
	LogDir             string
	DBType             string
	DBUser             string
	DBPassword         string
	AirflowVer         string
	PythonVer          string
}

func main() {
	cfg := GetUserInput()
	InstallPackages()
	SetupDatabase(cfg)
	SetupPythonEnv(cfg)
	GenerateConfigs(cfg)
	fmt.Println("Setup completed successfully!")
}

func trimNewline(s string) string {
	return strings.TrimRight(s, "\r\n")
}

func runCmd(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Command %s failed: %v", name, err)
	}
}

func InstallPackages() {
	fmt.Println("Installing system packages...")
	runCmd("sudo", "apt", "update")
	runCmd("sudo", "apt", "install", "-y", "python3-venv", "python3-dev", "libmariadb-dev", "build-essential", "mariadb-client-compat", "supervisor")
}

func GetUserInput() Config {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter Supervisor socket path (e.g., /var/run/supervisor.sock): ")
	sockPath, _ := reader.ReadString('\n')
	sockPath = trimNewline(sockPath)

	fmt.Print("Enter Supervisor log path (e.g., /var/log/supervisor/supervisord.log): ")
	logPath, _ := reader.ReadString('\n')
	logPath = trimNewline(logPath)

	fmt.Print("Enter Supervisor conf directory (e.g., /etc/supervisor/conf.d): ")
	supervisorDir, _ := reader.ReadString('\n')
	supervisorDir = trimNewline(supervisorDir)

	fmt.Print("Enter Airflow home path (e.g., /opt/airflow): ")
	airflowHome, _ := reader.ReadString('\n')
	airflowHome = trimNewline(airflowHome)

	fmt.Print("Enter Airflow user (e.g., root): ")
	airflowUser, _ := reader.ReadString('\n')
	airflowUser = trimNewline(airflowUser)

	fmt.Print("Enter Airflow API server port (e.g., 8080): ")
	airflowPort, _ := reader.ReadString('\n')
	airflowPort = trimNewline(airflowPort)

	fmt.Print("Enter Airflow logs directory (e.g., /opt/airflow/logs): ")
	logDir, _ := reader.ReadString('\n')
	logDir = trimNewline(logDir)

	fmt.Print("Choose backend DB (mysql/postgresql): ")
	dbType, _ := reader.ReadString('\n')
	dbType = trimNewline(dbType)

	fmt.Print("Enter DB user: ")
	dbUser, _ := reader.ReadString('\n')
	dbUser = trimNewline(dbUser)

	fmt.Print("Enter DB password: ")
	dbPassword, _ := reader.ReadString('\n')
	dbPassword = trimNewline(dbPassword)

	return Config{
		SupervisorSockPath: sockPath,
		SupervisorLogPath:  logPath,
		SupervisorDir:      supervisorDir,
		AirflowHome:        airflowHome,
		AirflowEnv:         airflowHome + "/airflow_env",
		AirflowUser:        airflowUser,
		AirflowPort:        airflowPort,
		LogDir:             logDir,
		DBType:             dbType,
		DBUser:             dbUser,
		DBPassword:         dbPassword,
		AirflowVer:         "3.0.3",
		PythonVer:          "3.12",
	}
}

func SetupDatabase(cfg Config) {
	if cfg.DBType == "mysql" {
		fmt.Println("Setting up MySQL DB and user...")
		runCmd("mysql", "-h", "127.0.0.1", "-P", "3306", "-u", "root", "-p"+cfg.DBPassword,
			"-e", "CREATE DATABASE IF NOT EXISTS airflow_db;")
		runCmd("mysql", "-h", "127.0.0.1", "-P", "3306", "-u", "root", "-p"+cfg.DBPassword,
			"-e", fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s';", cfg.DBUser, cfg.DBPassword))
		runCmd("mysql", "-h", "127.0.0.1", "-P", "3306", "-u", "root", "-p"+cfg.DBPassword,
			"-e", fmt.Sprintf("GRANT ALL PRIVILEGES ON airflow_db.* TO '%s'@'%%'; FLUSH PRIVILEGES;", cfg.DBUser))
	} else if cfg.DBType == "postgresql" {
		fmt.Println("Setting up PostgreSQL DB and user...")
		runCmd("psql", "-h", "127.0.0.1", "-p", "5432", "-U", "postgres", "-c",
			"CREATE DATABASE airflow_db;")
		runCmd("psql", "-h", "127.0.0.1", "-p", "5432", "-U", "postgres", "-c",
			fmt.Sprintf("CREATE USER %s WITH ENCRYPTED PASSWORD '%s';", cfg.DBUser, cfg.DBPassword))
		runCmd("psql", "-h", "127.0.0.1", "-p", "5432", "-U", "postgres", "-c",
			fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE airflow_db TO %s;", cfg.DBUser))
	} else {
		fmt.Println("Unsupported DB type")
	}
}

func SetupPythonEnv(cfg Config) {
	fmt.Println("Setting up Python virtual environment and installing Airflow...")
	runCmd("/usr/local/bin/python3.12", "-m", "venv", cfg.AirflowEnv)
	runCmd("bash", "-c", fmt.Sprintf("source %s/bin/activate && pip install --upgrade pip", cfg.AirflowEnv))

	os.Setenv("AIRFLOW_HOME", cfg.AirflowHome)
	os.Setenv("AIRFLOW_VERSION", cfg.AirflowVer)
	os.Setenv("PYTHON_VERSION", cfg.PythonVer)

	constraintURL := fmt.Sprintf("https://raw.githubusercontent.com/apache/airflow/constraints-%s/constraints-%s.txt", cfg.AirflowVer, cfg.PythonVer)
	runCmd("bash", "-c",
		fmt.Sprintf("source %s/bin/activate && pip install 'apache-airflow[%s]'=='%s' --constraint '%s'",
			cfg.AirflowEnv, cfg.DBType, cfg.AirflowVer, constraintURL))
}

func GenerateConfigs(cfg Config) {
	// Supervisor config
	supervisorTemplate := `
[unix_http_server]
file={{.SupervisorSockPath}}
chmod=0700

[supervisord]
logfile={{.SupervisorLogPath}}
pidfile=/var/run/supervisord.pid
childlogdir=/var/log/supervisor
user={{.AirflowUser}}

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

[supervisorctl]
serverurl=unix://{{.SupervisorSockPath}}

[include]
files = {{.SupervisorDir}}/*.conf
`
	t1 := template.Must(template.New("supervisor").Parse(supervisorTemplate))
	f1, _ := os.Create("supervisor.conf")
	defer f1.Close()
	t1.Execute(f1, cfg)

	// Airflow config
	airflowTemplate := `
[program:airflow-apiserver]
command={{.AirflowEnv}}/bin/airflow api-server --port {{.AirflowPort}}
directory={{.AirflowHome}}
user={{.AirflowUser}}
autostart=true
autorestart=true
startsecs=10
stopwaitsecs=20
stdout_logfile={{.LogDir}}/airflow-apiserver.out.log
stderr_logfile={{.LogDir}}/airflow-apiserver.err.log
environment=AIRFLOW_HOME="{{.AirflowHome}}"

[program:airflow-scheduler]
command={{.AirflowEnv}}/bin/airflow scheduler
directory={{.AirflowHome}}
user={{.AirflowUser}}
autostart=true
autorestart=true
startsecs=10
stopwaitsecs=20
stdout_logfile={{.LogDir}}/airflow-scheduler.out.log
stderr_logfile={{.LogDir}}/airflow-scheduler.err.log
environment=AIRFLOW_HOME="{{.AirflowHome}}"

[program:airflow-dag-processor]
command={{.AirflowEnv}}/bin/airflow dag-processor
directory={{.AirflowHome}}
user={{.AirflowUser}}
autostart=true
autorestart=true
startsecs=10
stopwaitsecs=20
stdout_logfile={{.LogDir}}/airflow-dag-processor.out.log
stderr_logfile={{.LogDir}}/airflow-dag-processor.err.log
environment=AIRFLOW_HOME="{{.AirflowHome}}"

[program:airflow-triggerer]
command={{.AirflowEnv}}/bin/airflow triggerer
directory={{.AirflowHome}}
user={{.AirflowUser}}
autostart=true
autorestart=true
startsecs=10
stopwaitsecs=20
stdout_logfile={{.LogDir}}/airflow-triggerer.out.log
stderr_logfile={{.LogDir}}/airflow-triggerer.err.log
environment=AIRFLOW_HOME="{{.AirflowHome}}"
`
	t2 := template.Must(template.New("airflow").Parse(airflowTemplate))
	f2, _ := os.Create("airflow.conf")
	defer f2.Close()
	t2.Execute(f2, cfg)
}
