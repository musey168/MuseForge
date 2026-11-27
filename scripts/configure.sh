#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
python3 - "$@" <<'PY'
import os,secrets,pathlib,shlex,base64,datetime,re,stat,sys,tempfile
p=pathlib.Path('.env')
data_dir=os.path.abspath(os.environ.get('LOOPFORGE_DATA_DIR','.data'))
project_key_name='AGENTFLOW_PROJECT_CREDENTIAL_KEY'
project_key_only=sys.argv[1:] == ['--project-credential-key']
def random_credential_key():
 return base64.b64encode(secrets.token_bytes(32)).decode()
def credential_keys():
 # AES-256 key sealing chat Bot secrets; keep it with the database backups.
 return f"k{datetime.date.today():%Y%m}:{random_credential_key()}"
def project_credential_key():
 return os.environ.get(project_key_name) or random_credential_key()

def sqlite_dsn():
 # One SQLite file in the data directory. The platform creates the file and
 # its schema on first start; no database server is involved.
 return f'file:{os.path.join(data_dir, "museforge.db")}'

def mysql_installation():
 # An installation that names a MySQL DSN, or enables the MySQL compose
 # profile, keeps running on MySQL. Everything else runs on one SQLite file.
 if os.environ.get('AGENTFLOW_MYSQL_DSN','').strip():
  return True
 return 'mysql' in [profile.strip() for profile in os.environ.get('COMPOSE_PROFILES','').split(',')]

def datasource_dsn():
 # The datasource the platform uses. An empty value means "use the MySQL
 # DSN", which a MySQL installation names in AGENTFLOW_MYSQL_DSN.
 return '' if mysql_installation() else sqlite_dsn()

def env_value(text, name):
 match = re.search(rf'(?m)^[ \t]*(?:export[ \t]+)?{name}[ \t]*=(.*)$', text)
 if not match:
  return ''
 return match.group(1).strip().strip('"\'')

def configure_project_credential(text):
 matches = list(re.finditer(rf'(?m)^[ \t]*(?:export[ \t]+)?{project_key_name}[ \t]*=([^\r\n]*)', text))
 if not matches:
  return text + f'\n{project_key_name}={shlex.quote(project_credential_key())}\n'
 match = matches[-1]
 empty = re.fullmatch(r"""[ \t]*(?:""|'')?(?:[ \t]+(#.*))?[ \t]*""", match[1])
 if empty is None:
  return text
 value = shlex.quote(project_credential_key())
 if empty[1]:
  value += ' ' + empty[1]
 return text[:match.start(1)] + value + text[match.end(1):]

def save_existing_env(text):
 target = p.resolve()
 mode = stat.S_IMODE(target.stat().st_mode)
 fd, temporary = tempfile.mkstemp(prefix=f'{target.name}.', dir=target.parent)
 try:
  with os.fdopen(fd, 'w', newline='') as env:
   env.write(text)
   os.fchmod(env.fileno(), mode)
  os.replace(temporary, target)
 finally:
  pathlib.Path(temporary).unlink(missing_ok=True)

if p.exists():
 with p.open(newline='') as env:
  text = env.read()
 added=[]
 if not project_key_only:
  if 'LOOPFORGE_DATA_DIR=' not in text:
   text += f'\nLOOPFORGE_DATA_DIR={shlex.quote(data_dir)}\n'
   added.append('LOOPFORGE_DATA_DIR')
  # A fresh installation runs on one SQLite file. An installation that
  # already names a datasource keeps it: MySQL stays supported.
  if 'AGENTFLOW_DATASOURCE_DSN=' not in text and 'AGENTFLOW_MYSQL_DSN=' not in text:
   text += f'\nAGENTFLOW_DATASOURCE_DSN={shlex.quote(datasource_dsn())}\n'
   added.append('AGENTFLOW_DATASOURCE_DSN')
  # A MySQL installation needs its server; a SQLite one does not.
  if 'COMPOSE_PROFILES=' not in text and env_value(text, 'AGENTFLOW_MYSQL_DSN'):
   text += '\nCOMPOSE_PROFILES=platform,mysql\n'
   added.append('COMPOSE_PROFILES')
  if 'AGENTFLOW_CREDENTIAL_KEYS=' not in text:
   text += f'\nAGENTFLOW_CREDENTIAL_KEYS={shlex.quote(credential_keys())}\n'
   added.append('AGENTFLOW_CREDENTIAL_KEYS')
 configured = configure_project_credential(text)
 if configured != text:
  added.append(project_key_name)
 if added:
  save_existing_env(configured)
 print('Configured '+', '.join(added)+' in existing .env' if added else 'Keeping existing .env')
elif project_key_only:
 sys.exit('Not installed yet. Run ./start.sh first.')
else:
 password=secrets.token_hex(24)
 http_port=os.environ.get('AGENTFLOW_HTTP_PORT','8090');mysql_port=os.environ.get('LOOPFORGE_MYSQL_PORT','33315')
 data={'LOOPFORGE_DB_PASSWORD':password,'LOOPFORGE_DB_ROOT_PASSWORD':secrets.token_hex(24),'LOOPFORGE_MYSQL_PORT':mysql_port,'COMPOSE_PROJECT_NAME':os.environ.get('COMPOSE_PROJECT_NAME','loopforge'),'AGENTFLOW_HTTP_PORT':http_port,'AGENTFLOW_DATASOURCE_DSN':datasource_dsn(),'AGENTFLOW_MYSQL_DSN':(f'loopforge:{password}@tcp(127.0.0.1:{mysql_port})/loopforge' if mysql_installation() else ''),'AGENTFLOW_SESSION_SECRET':secrets.token_hex(32),'AGENTFLOW_CREDENTIAL_KEYS':credential_keys(),'AGENTFLOW_AUTH_REQUIRED':'true','LOOPFORGE_DATA_DIR':data_dir,'LOOPFORGE_BIND':os.environ.get('LOOPFORGE_BIND','0.0.0.0'),'LOOPFORGE_PUBLIC_URL':os.environ.get('LOOPFORGE_PUBLIC_URL',''),'LOOPFORGE_IMAGE':os.environ.get('LOOPFORGE_IMAGE',''),'AGENTFLOW_GITHUB_CLIENT_ID':os.environ.get('AGENTFLOW_GITHUB_CLIENT_ID','')}
 data[project_key_name]=project_credential_key()
 # The database server is only part of a MySQL installation.
 data['COMPOSE_PROFILES']='platform,mysql' if mysql_installation() else 'platform'
 fd=os.open(str(p),os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'w') as f: f.write(''.join(f'{k}={shlex.quote(v)}\n' for k,v in data.items()))
 print('Created .env with unique local secrets')
PY
