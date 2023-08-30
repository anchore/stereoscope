CONFIG_DIR = config
BIN_DIR = bin

TEST_BIN = $(BIN_DIR)/run-test

CERTS_DIR = $(CONFIG_DIR)/certs
CA_CERT_FILE = $(CERTS_DIR)/server.crt
CA_KEY_FILE = $(CERTS_DIR)/server.key
CLIENT_CSR_FILE = $(CERTS_DIR)/client.csr
CLIENT_KEY_FILE = $(CERTS_DIR)/client.key
CLIENT_CERT_FILE = $(CERTS_DIR)/client.crt
AUTH_FILE = $(CONFIG_DIR)/auth/htpasswd

# registry credentials
USERNAME = testuser42
PASSWORD = testpass42

CN = registry.null
REGISTRY_HOST = $(CN):5000
REGISTRY_URL = https://$(REGISTRY_HOST)/v2/
IMAGE = $(REGISTRY_HOST)/busybox:latest
IMAGE_FILE = image.tar.gz

all: clean-bin start

$(CERTS_DIR):
	mkdir -p $(CERTS_DIR)

$(CA_CERT_FILE): $(CERTS_DIR)
	openssl req -x509 \
		-newkey rsa:4096 \
		-keyout $(CA_KEY_FILE) \
		-out $(CA_CERT_FILE) \
		-sha256 -days 3650 -nodes \
		-addext "subjectAltName = DNS:$(CN)" \
		-subj "/C=XX/ST=StateName/L=CityName/O=CompanyName/OU=CompanySectionName/CN=$(CN)"

$(CLIENT_CERT_FILE): $(CERTS_DIR)
	openssl genpkey -algorithm RSA -out $(CLIENT_KEY_FILE)
	openssl req -new \
		-key $(CLIENT_KEY_FILE) \
		-out $(CLIENT_CSR_FILE) \
		-addext "subjectAltName = DNS:$(CN)" \
		-subj "/C=XX/ST=StateName/L=CityName/O=CompanyName/OU=CompanySectionName/CN=$(CN)"
	openssl x509 -req \
		-in $(CLIENT_CSR_FILE) \
		-CAcreateserial \
		-CA $(CA_CERT_FILE) \
		-CAkey $(CA_KEY_FILE) \
		-out $(CLIENT_CERT_FILE) \
		-days 1000 \
		-sha256

$(AUTH_FILE):
	mkdir -p $(dir $(AUTH_FILE))
	htpasswd -Bbn $(USERNAME) $(PASSWORD) > $(AUTH_FILE)

$(CONFIG_DIR): $(CERTS_DIR) $(AUTH_FILE)

.PHONY: config
config: $(CA_CERT_FILE) $(AUTH_FILE) $(CLIENT_CERT_FILE)

.PHONY: test-auth
test-auth:
	curl --cacert $(CA_CERT_FILE) -i -H 'Authorization: Basic $(shell bash -c 'echo -n "$(USERNAME):$(PASSWORD)" | base64')' $(REGISTRY_URL)

.PHONY: start
start: $(CA_CERT_FILE) $(AUTH_FILE) $(CLIENT_CERT_FILE) $(TEST_BIN)
	docker compose up -d registry
	docker compose up loader --exit-code-from loader

.PHONY: run
run:
	docker compose up runner --exit-code-from runner

.PHONY: stop
stop:
	docker compose down

.PHONY: load
load:
	crane pull busybox:latest $(IMAGE_FILE)
	crane auth login $(REGISTRY_HOST) -u $(USERNAME) -p $(PASSWORD)
	crane push --insecure $(IMAGE_FILE) $(IMAGE)

$(TEST_BIN):
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(TEST_BIN) .

.PHONY: clean
clean: stop clean-bin
	rm -rf $(CONFIG_DIR)

clean-bin:
	rm -rf $(BIN_DIR)