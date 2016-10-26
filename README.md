### Описание установки

* Необходимые зависимости:

```
make cmake pkg-config libssh2-1 libssh2-1-dev
```

* устанавливаем динамическую библиотеку, реализующую C-API для работы с `git`:

```
$ wget https://github.com/libgit2/libgit2/archive/v0.24.0.tar.gz && tar xzf libgit2-0.24.0.tgz && cd libgit2-0.24.0
$ cmake -DCMAKE_INSTALL_PREFIX=/usr .
$ make && checkinstall
$ dpkg -i libgit2-0.24.0.deb
```

* подготавливаем дерево каталогов для сборки проекта и выполняем сборку:

```
$ mkdir -p /tmp/go-gitlab/src
$ go get 
$ export GOPATH=/tmp/go-gitlab
$ go build
```

### Подготовка конфигурации

Конфигурационный файл по умолчанию располагается по пути: /etc/githook.conf 

Описание конфигурационных директив:

* global - глобальная конфигурация сервера. Содержит параметры для вызовов bind(2) && listen(2), а также параметры вывода отладочной информации в лог, и пользователя для вызова setuid(2) при старте.

* web - параметры веб-интерфейса. Директории для запросов, а также путь к шаблонам. Путь к шаблонам будет определяться как указанный + html. Параметры `api` и `management` ни в коем случае не должны быть эквивалентны

* logger - параметры сисмемы логирования и отправки отчетов. `skypeUrl` - адрес, по которому будет отправлен запрос с параметрами '?user=<skypeDistination>&message=<message from system>'

* gitlab - параметры для доступа к api системы GitLab. Используется для перевода id пользователя в имя из присылаемых отчетов на систему от GitLab. Token можно получить в профиле пользователя в GitLab. Схема для запросов модет быть либо `http`, либо `https`

* git - параметры для обращения к git-серверу. Должны быть по аналогии с настройками для работы с git из shell. Ключи, предоставляемые как приватные не должны быть зашифрованны, т.к. зашифрованные ключи (пр. id-rsa) системой распознанны не будут

* секции repository - рядом с секцией ставится уникальное имя. Оно не обязательно должно соответствовать названию репозитория или ветки, и может принимать любое значение. Path - каталог в который будет скачан репозиторий, который будет сопровождаться в дальнейшем. В него выкачивается только ветка, указанная в данной секции как branch. Remote - ssh-адрес для обращения. Следует обратить внимание, что формат не стандартный. Например в gitlab и на github такой адрес записывается как: ssh://git@gitlab.ru:user/repo.git, в то время как в конфигурацию он должен быть записан как: ssh://git@gitlab.ru*/*user/repo.git. PushRequests - закачивать изменения из репозитория при получении событий о push. MergeRequest - закачивать изменения из репозитория при получении события о merge_[request|accept|closed]. Notifications - отправлять нотификации о событии (по умолчанию "тихий режим")

Example:

``` ini
[global]
port = 8189 ; listen web-interface port
host = 127.0.0.1 ; bind address
debug = true ; debug log
user = root ; run daemon from user (need root grants)

[web]
api = /api ; page for listen request from gitlab
management = /admin ; management page
templates = /www/templates ; full templates path

[logger]
skypeUrl = http://skypebot.ru/skype.php ; url for skype api interface 
skypeDistination = user ; send skype message to user (system messages)

[gitlab]
host = gitlab.ru ; gitlab host
scheme = http ; gitlab api schema - can be http or https
user = gitlab_user ; user for send api request to gitlab
token = gitlab_token ; token for auth user for auth in gitlab

[git]
publicKey = /home/user/.ssh/key.pub ; public key for fetching reposytory via ssh
privateKey = /home/user/.ssh/key.key ; private key - should be without cripto
user = git ; user for auth via ssh to git

[repository "Development"]
path = /tmp/repos ; path for managment with repo "Development"
branch = master ; branch for checkout and monitoring
remote = ssh://git@gitlab.ru/user/repo.git ; url to the remote repo - should be in ssh schema only
pushRequests = true
mergeRequests = true
notifications = true
```

### Параметры запуска

Для запуска используется синтаксис:

```
go-gitlab:
  -config="/etc/githooks.conf": путь до конфигурационного файла, отличного от предлагаемого по умолчанию
  -daemon=false: демонизироваться при старте (по умолчанию параметр выставлен в ложное значение)
  -log="/var/log/githooks.log": путь до лог-файла. По умолчанию выставлен в "/var/log/githooks.log"
  -pid="/var/run/githooks.pid": путь до pid-файла. По умолчанию выставлен в "/var/log/githooks.pid"
```

Для запуска даемона необходимо указывать полный путь до бинарного файла и до файла конфигурации. Как пример:

```
$ /usr/local/sbin/go-gitlab -config=/usr/share/go-gitlab/gitlab.conf -daemon
```

### Описание функционала

Процесс состоит из нескольких подпроцессов (goroutines), общающихся между собой по внутренним каналам.
Сразу при запуске запускается сопроцесс для перехвата системных вызовов, реализующий корректное завершение всех остальных сопроцессов.
При инициализации или открытии уже инициализированного каталога с репозиторием запускается сопроцесс мониторинга изменений в каталоге.
Под каждую директиву `repository` запускается сопроцесс принятия уведомления об изменениях, их анализа, отправки уведомлений и непосредственного изменения файлов.
Запускается сопроцесс обработки команд по websocket и реализации подписки на уведомления по websocket.
После этого запускается сопроцесс веб-интерфейса (менеджмент и Api).

### Features
> * система отката в секции info
> * отказ от обызательности указания полного пути до исполняемого файла при запуске
> * исправления использования бинарной команды `git merge` в коде
> * реализация возможности отправки сообщения на email
> * сброс последней ошибки из интерфейса
> * внесение изменений на лету в список коммитов и ошибок (websocket)
