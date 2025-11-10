#!/bin/bash
# Generate TLS certificates for StreamBus testing
# DO NOT USE IN PRODUCTION - Use proper CA-signed certificates

set -e

CERT_DIR="./certs"
VALIDITY_DAYS=365

echo "Creating certificate directory..."
mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

# Generate CA private key and certificate
echo "Generating CA certificate..."
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days $VALIDITY_DAYS -key ca.key -out ca.crt \
  -subj "/C=US/ST=California/L=SanFrancisco/O=StreamBus/OU=Testing/CN=StreamBus-CA"

# Generate broker private key and certificate signing request
echo "Generating broker certificate..."
openssl genrsa -out broker.key 2048
openssl req -new -key broker.key -out broker.csr \
  -subj "/C=US/ST=California/L=SanFrancisco/O=StreamBus/OU=Broker/CN=localhost"

# Sign broker certificate with CA
openssl x509 -req -days $VALIDITY_DAYS -in broker.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out broker.crt

# Generate client private key and certificate (for mTLS)
echo "Generating client certificate..."
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr \
  -subj "/C=US/ST=California/L=SanFrancisco/O=StreamBus/OU=Client/CN=admin-client"

# Sign client certificate with CA
openssl x509 -req -days $VALIDITY_DAYS -in client.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out client.crt

# Generate producer service certificate
echo "Generating producer service certificate..."
openssl genrsa -out producer.key 2048
openssl req -new -key producer.key -out producer.csr \
  -subj "/C=US/ST=California/L=SanFrancisco/O=StreamBus/OU=Producer/CN=producer-service"

openssl x509 -req -days $VALIDITY_DAYS -in producer.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out producer.crt

# Generate consumer service certificate
echo "Generating consumer service certificate..."
openssl genrsa -out consumer.key 2048
openssl req -new -key consumer.key -out consumer.csr \
  -subj "/C=US/ST=California/L=SanFrancisco/O=StreamBus/OU=Consumer/CN=consumer-service"

openssl x509 -req -days $VALIDITY_DAYS -in consumer.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out consumer.crt

# Clean up CSR files
rm -f *.csr *.srl

# Set proper permissions
chmod 600 *.key
chmod 644 *.crt

echo ""
echo "Certificate generation complete!"
echo "Files created in $CERT_DIR:"
echo "  - ca.crt, ca.key (Certificate Authority)"
echo "  - broker.crt, broker.key (Broker certificate)"
echo "  - client.crt, client.key (Client certificate for admin)"
echo "  - producer.crt, producer.key (Producer service certificate)"
echo "  - consumer.crt, consumer.key (Consumer service certificate)"
echo ""
echo "To verify a certificate:"
echo "  openssl x509 -in broker.crt -text -noout"
echo ""
echo "To verify certificate chain:"
echo "  openssl verify -CAfile ca.crt broker.crt"
echo ""
echo "REMINDER: These certificates are for TESTING only."
echo "Use proper CA-signed certificates in production!"
