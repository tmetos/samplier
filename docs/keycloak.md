# Keycloak setup for Samplier

Samplier validates OIDC bearer tokens against Keycloak's JWKS endpoint.

## Local development

Keycloak starts with `make up` at <http://localhost:8081> (admin / admin).

Create a realm, a public client, and a user:

```bash
KC=http://localhost:8081
ADMIN=$(curl -s -X POST "$KC/realms/master/protocol/openid-connect/token" \
  -d 'grant_type=password&client_id=admin-cli&username=admin&password=admin' \
  | jq -r .access_token)

# Realm
curl -s -X POST "$KC/admin/realms" -H "Authorization: Bearer $ADMIN" \
  -H 'Content-Type: application/json' \
  -d '{"realm":"samplier","enabled":true}'

# Public client with direct access grants (password flow) for CLI testing
curl -s -X POST "$KC/admin/realms/samplier/clients" -H "Authorization: Bearer $ADMIN" \
  -H 'Content-Type: application/json' \
  -d '{"clientId":"samplier-ui","publicClient":true,"directAccessGrantsEnabled":true,
       "redirectUris":["http://localhost:5173/*"],"webOrigins":["http://localhost:5173"]}'

# A user
curl -s -X POST "$KC/admin/realms/samplier/users" -H "Authorization: Bearer $ADMIN" \
  -H 'Content-Type: application/json' \
  -d '{"username":"analyst","enabled":true,"email":"analyst@example.com",
       "emailVerified":true,
       "credentials":[{"type":"password","value":"analyst123","temporary":false}]}'
```

Obtain a token and call the API:

```bash
TOKEN=$(curl -s -X POST \
  "$KC/realms/samplier/protocol/openid-connect/token" \
  -d 'grant_type=password&client_id=samplier-ui&username=analyst&password=analyst123' \
  | jq -r .access_token)

curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/projects
```

Run the API with auth enabled:

```bash
AUTH_ENABLED=true \
OIDC_ISSUER=http://localhost:8081/realms/samplier \
OIDC_CLIENT_ID=samplier-ui \
make run
```

## How a token is validated

1. `Authorization: Bearer <jwt>` is parsed (RS256 only).
2. The signing key is selected by `kid` from
   `<issuer>/protocol/openid-connect/certs` (cached 15 minutes).
3. The signature is verified with RSA-PKCS1v15 + SHA-256.
4. `iss` must equal `OIDC_ISSUER` exactly.
5. The client is accepted when `aud` **or** `azp` equals `OIDC_CLIENT_ID`
   (Keycloak sets `aud: account` and `azp: <client>` for public clients).
6. `exp` must be in the future.

## Optional: add a proper audience

If you prefer `aud` to carry the client id, add an audience mapper:

```bash
curl -s -X POST "$KC/admin/realms/samplier/clients/$( \
    curl -s -H "Authorization: Bearer $ADMIN" \
      "$KC/admin/realms/samplier/clients?clientId=samplier-ui" | jq -r '.[0].id')/protocol-mappers/models" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"name":"audience","protocol":"openid-connect","protocolMapper":"oidc-audience-mapper",
       "config":{"included.client.audience":"samplier-ui","access.token.claim":"true"}}'
```

## Roles

Realm roles are read from `realm_access.roles`. `auth.RequireRole("admin")`
middleware can gate privileged endpoints; `project_member` in Postgres carries
per-project roles (viewer/editor/admin).

## Production

- Set `KC_HOSTNAME=https://your-domain/auth` and `KC_HTTP_RELATIVE_PATH=/auth`.
- Use `start --optimized` with an external Postgres (`KC_DB=postgres`).
- Terminate TLS at the load balancer and forward `X-Forwarded-*`
  (`KC_PROXY_HEADERS=xforwarded`).
- Store the admin credentials in Secrets Manager / Secret Manager (the Terraform
  modules do this).
