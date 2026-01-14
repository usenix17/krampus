# Krampus - Santa Sync Server

A full-featured Santa sync server with OIDC authentication, user voting system for binary rules, and a Material-UI web portal. Single binary deployment with embedded React frontend.

![Login](docs/login.png)


![Dashboard](docs/dashboard.png)


![Rules](docs/rules.png)


![Events](docs/events.png)


## Features

- **OIDC Authentication**: Generic OIDC provider support (Authentik, Google, Okta, Auth0, Keycloak, etc.) with HS256 and RS256 support
- **Material-UI Web Portal**: Modern React-based interface for managing all aspects of the server
- **Voting System**: Users can vote to allowlist or blocklist binaries with configurable threshold
- **Role-Based Access Control**: Admin and user roles with different permissions
- **Machine Management**: Register Santa clients, generate mobileconfig profiles, and delete machines
- **Event Tracking**: View execution events from all enrolled machines
- **Program Analytics**: Aggregate view of all executed binaries with metadata
- **Santa Sync Protocol**: Full implementation of the Santa sync protocol (preflight, event upload, rule download, postflight)
- **RESTful API**: Comprehensive API for managing proposals, rules, machines, events, and users
- **SQLite Database**: Lightweight database for rules, votes, events, and session storage
- **Single Binary Deployment**: Embedded frontend with no external dependencies

## Architecture

### Backend (Go)
- **Framework**: Gin web framework
- **Database**: SQLite with comprehensive schema
- **Authentication**: OIDC (HS256/RS256) + JWT tokens with httpOnly cookies
- **Services**: OIDC, JWT, Voting, Mobileconfig generation, Santa sync protocol
- **Middleware**: Auth validation, admin role checking, CORS
- **Static Files**: Embedded React frontend via Go embed

### Frontend (React)
- **Framework**: React 18+ with Vite
- **UI Library**: Material-UI v5
- **Routing**: React Router v6
- **State**: Context API for auth state
- **HTTP Client**: Axios with cookie-based authentication
- **Pages**: Dashboard, Proposals, Rules, Machines, Events, Programs, Users (admin)

## Quick Start

### Prerequisites
- Go 1.23+ or higher
- Node.js 18+ and npm (for building the frontend)
- An OIDC provider (Authentik, Google, Okta, Auth0, Keycloak, etc.)

### Installation

1. **Clone the repository**
   ```bash
   cd krampus
   ```

2. **Configure environment variables**
   ```bash
   cp .env.example .env
   ```
   Edit `.env` and configure your OIDC provider details:
   - `OIDC_PROVIDER_URL`: Your OIDC provider's URL
   - `OIDC_CLIENT_ID`: Your application's client ID
   - `OIDC_CLIENT_SECRET`: Your application's client secret
   - `OIDC_REDIRECT_URL`: Callback URL (default: http://localhost:8080/auth/callback)
   - `ADMIN_EMAILS`: Comma-separated list of admin emails

3. **Build the application**

   Using the Makefile:
   ```bash
   make all
   ```

   Or manually:
   ```bash
   # Build frontend
   cd client && npm install && npm run build

   # Copy frontend to server static directory
   cp -r dist/* ../server/static/

   # Build Go binary with embedded frontend
   cd .. && go build -ldflags="-s -w" -o krampus-server ./server
   ```

4. **Run the server**
   ```bash
   ./krampus-server
   ```

   The server will start on port 8080 (configurable via `SERVER_PORT` env var). Access the web UI at http://localhost:8080

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OIDC_PROVIDER_URL` | OIDC provider URL | - |
| `OIDC_CLIENT_ID` | OIDC client ID | - |
| `OIDC_CLIENT_SECRET` | OIDC client secret | - |
| `OIDC_REDIRECT_URL` | OIDC callback URL | `http://localhost:8080/auth/callback` |
| `OIDC_SCOPES` | OIDC scopes (comma-separated) | `openid,profile,email` |
| `JWT_SECRET` | Secret for signing JWT tokens | `change-me-in-production` |
| `JWT_EXPIRY` | JWT token expiration duration | `24h` |
| `VOTE_THRESHOLD` | Number of votes needed to approve a proposal | `3` |
| `ADMIN_EMAILS` | Admin emails (comma-separated) | - |
| `SYNC_BASE_URL` | Base URL for Santa clients | `http://localhost:8080` |
| `SERVER_PORT` | Server port | `8080` |
| `DATABASE_PATH` | SQLite database file path | `./database/krampus.db` |

### OIDC Provider Setup

#### Google OAuth2
1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select existing
3. Enable Google+ API
4. Create OAuth 2.0 credentials
5. Add authorized redirect URI: `http://localhost:8080/auth/callback`
6. Use the client ID and secret in your `.env` file

#### Keycloak
1. Create a new realm and client
2. Set redirect URI: `http://localhost:8080/auth/callback`
3. Enable "Standard Flow" and "Direct Access Grants"
4. Use `http://your-keycloak-server/realms/your-realm` as `OIDC_PROVIDER_URL`

#### Authentik
1. Create a new OAuth2/OpenID Provider
2. Set redirect URI: `http://localhost:8080/auth/callback`
3. Configure signing algorithm (HS256 or RS256)
4. Create an application and link it to the provider
5. Use `https://your-authentik-server/application/o/your-app/.well-known/openid-configuration` as discovery URL
6. Set `OIDC_PROVIDER_URL` to `https://your-authentik-server/application/o/your-app/`

## API Endpoints

### Authentication
- `GET /auth/login` - Initiate OIDC login flow
- `GET /auth/callback` - OIDC callback handler
- `POST /auth/logout` - Logout (revoke JWT session)
- `GET /auth/me` - Get current user info (requires auth)

### Proposals
- `GET /api/proposals` - List all proposals (filter by `?status=PENDING`)
- `GET /api/proposals/:id` - Get proposal details
- `POST /api/proposals` - Create new proposal
- `POST /api/proposals/:id/vote` - Vote on proposal
- `POST /api/proposals/:id/approve` - Admin: Approve proposal (bypass voting)
- `DELETE /api/proposals/:id` - Delete proposal (creator or admin)

### Rules
- `GET /api/rules` - List all rules (filter by `?policy=ALLOWLIST` or `?rule_type=BINARY`)
- `GET /api/rules/:id` - Get rule details
- `POST /api/rules` - Admin: Create rule directly
- `DELETE /api/rules/:id` - Admin: Delete rule

### Machines
- `GET /api/machines` - List all enrolled machines
- `GET /api/machines/:id` - Get machine details
- `POST /api/machines` - Register new machine
- `POST /api/machines/:id/mobileconfig` - Generate mobileconfig profile
- `DELETE /api/machines/:id` - Admin: Delete machine

### Events
- `GET /api/events` - List execution events (filter by `?machine_id=` or `?decision=ALLOW`)
- Pagination: `?page=1&limit=50`

### Programs
- `GET /api/programs` - List aggregated program statistics
- Shows unique binaries with execution counts, allow/block stats, and metadata

### Users
- `GET /api/users` - Admin: List all users
- `GET /api/users/:id` - Admin: Get user details
- `PUT /api/users/:id` - Admin: Update user role
- `DELETE /api/users/:id` - Admin: Delete user

### Santa Sync Protocol
- `POST /preflight/:machine_id` - Preflight sync stage
- `POST /eventupload/:machine_id` - Event upload stage
- `POST /ruledownload/:machine_id` - Rule download stage
- `POST /postflight/:machine_id` - Postflight sync stage

## Web Portal

Access the Material-UI web portal at `http://localhost:8080` after starting the server. The portal includes:

### Dashboard
- Overview statistics (proposals, rules, machines, users)
- Quick access to all sections

### Proposals
- View all proposals with voting status
- Create new proposals for binaries
- Vote on proposals (ALLOWLIST or BLOCKLIST)
- Admin: Approve proposals bypassing voting

### Rules
- View all active allowlist/blocklist rules
- Filter by policy or rule type
- Admin: Create and delete rules directly

### Machines
- View all enrolled Santa clients
- Register new machines
- Generate and download mobileconfig profiles
- Admin: Delete machines

### Events
- View execution events from all machines
- Filter by machine, decision (ALLOW/BLOCK)
- Pagination support

### Programs
- Aggregated view of all executed binaries
- Execution statistics and metadata
- Certificate and signing information

### Users (Admin Only)
- Manage user accounts
- Update user roles
- Delete users

## API Usage Examples

### Create a Proposal
```bash
curl -X POST http://localhost:8080/api/proposals \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "identifier": "a1b2c3d4e5f6...",
    "rule_type": "BINARY",
    "proposed_policy": "BLOCKLIST",
    "custom_message": "Malware detected"
  }'
```

### Vote on a Proposal
```bash
curl -X POST http://localhost:8080/api/proposals/1/vote \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "vote_type": "BLOCKLIST"
  }'
```

### Register a Machine
```bash
curl -X POST http://localhost:8080/api/machines \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "machine_id": "MACHINE123",
    "serial_number": "C02ABC123XYZ"
  }'
```

### Generate Mobileconfig
```bash
curl -X POST http://localhost:8080/api/machines/MACHINE123/mobileconfig \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "client_mode": "LOCKDOWN",
    "upload_interval": 600
  }' \
  --output MACHINE123.mobileconfig
```

## Voting System

The voting system allows users to collectively decide on binary allowlist/blocklist rules:

1. **Create Proposal**: Any authenticated user can create a proposal for a binary
2. **Vote**: Users vote ALLOWLIST or BLOCKLIST on the proposal
3. **Threshold**: When votes reach the configured threshold (default: 3), the proposal auto-finalizes
4. **Rule Creation**: The winning policy (most votes) becomes an active rule
5. **Admin Override**: Admins can bypass voting and directly approve proposals

### Proposal Lifecycle
- **PENDING**: Waiting for votes
- **APPROVED**: Threshold reached, rule created
- **REJECTED**: Admin rejected (not currently auto-rejected)

## Santa Client Configuration

### Prerequisites

1. **Install Santa** from NorthPole Security (Google's Santa was sunset in Feb 2025):
   ```bash
   # Download latest Santa release from NorthPole Security
   # https://github.com/northpolesec/santa/releases

   # Or using Homebrew (if available)
   brew install --cask santa
   ```

2. **Verify Santa is running**:
   ```bash
   sudo santactl status
   ```

### Deploy Configuration via Mobileconfig (Recommended)

Mobileconfig profiles provide the easiest deployment method and automatically configure Santa:

1. **Register machine** in the Krampus web portal (Machines page)
   - Click "Register Machine"
   - Enter a unique Machine ID (e.g., hostname or serial number)
   - Optionally provide serial number and other metadata

2. **Generate mobileconfig** for the machine
   - Click "Download Config" on the machine card
   - Configure client mode:
     - **MONITOR**: Logs blocks but allows execution (testing/audit)
     - **LOCKDOWN**: Enforces blocks (production)
   - Set sync interval (default: 600 seconds)
   - Download the `.mobileconfig` file

3. **Install the profile** on the target Mac:
   ```bash
   # Double-click the .mobileconfig file, or:
   sudo /usr/bin/profiles install -path /path/to/MACHINE123.mobileconfig
   ```

   - This will prompt for admin credentials
   - The profile configures Santa with your server URL and settings

4. **Verify configuration**:
   ```bash
   sudo santactl status
   # Should show:
   # - Sync server URL pointing to your Krampus server
   # - Client mode (MONITOR or LOCKDOWN)
   # - Last sync time
   ```

5. **Trigger initial sync**:
   ```bash
   sudo santactl sync
   ```

### What the Mobileconfig Configures

The generated mobileconfig automatically sets:
- **SyncBaseURL**: Your Krampus server URL (HTTPS enforced)
- **ClientMode**: MONITOR (1) or LOCKDOWN (2)
- **MachineID**: Unique identifier for this client
- **FullSyncInterval**: How often to sync with server
- **EventDetailURL**: URL for "Request Access" button when binaries are blocked
- **EnableAllEventUpload**: Upload all execution events to server
- **EnableBundles**: Support for bundle-based rules

### Verifying Santa Sync

After installation, check Santa is communicating with your server:

```bash
# Check Santa status
sudo santactl status

# Check recent sync logs
log show --predicate 'subsystem == "com.northpolesec.santa"' --last 5m

# Force a sync
sudo santactl sync
```

You should see the machine appear in the Krampus web portal under "Machines" with updated sync timestamps.

### Removing the Configuration

To remove the Krampus configuration profile:

```bash
# List installed profiles
sudo /usr/bin/profiles list

# Remove the Santa configuration profile
sudo /usr/bin/profiles remove -identifier com.northpolesec.santa.config.<UUID>
```

**Note**: NorthPole Security maintains Santa after Google's sunset in February 2025. Visit [https://github.com/northpolesec/santa](https://github.com/northpolesec/santa) for installation and documentation.

## Database Schema

### Core Tables
- **users**: User accounts with OIDC subjects and roles
- **proposals**: Binary proposals being voted on
- **votes**: Individual user votes on proposals
- **rules**: Active allowlist/blocklist rules
- **machines**: Enrolled Santa clients
- **events**: Execution events from Santa clients
- **sessions**: JWT session tracking for revocation

## Development

### Project Structure
```
krampus/
├── server/
│   ├── main.go                 # Entry point with embedded static files
│   ├── config/                 # Configuration management
│   ├── database/               # Database connection and migrations
│   ├── models/                 # Data models
│   ├── handlers/               # HTTP handlers (auth, proposals, rules, etc.)
│   ├── middleware/             # Auth and admin middleware
│   ├── services/               # Business logic (OIDC, JWT, voting, mobileconfig)
│   └── static/                 # Embedded frontend build artifacts
├── client/
│   ├── src/
│   │   ├── pages/              # Dashboard, Proposals, Rules, Machines, etc.
│   │   ├── components/         # Layout, auth components
│   │   ├── contexts/           # AuthContext
│   │   └── api/                # API client
│   ├── public/                 # Static assets (logo)
│   ├── package.json
│   └── vite.config.js
├── database/                   # SQLite database file
├── .env                        # Environment configuration
├── Makefile                    # Build automation
└── README.md
```

### Adding New Features

1. **Add Model**: Create model in `server/models/`
2. **Add Migration**: Update `server/database/migrations.go`
3. **Add Handler**: Create handler in `server/handlers/`
4. **Add Route**: Register route in `server/main.go`
5. **Add Service**: Add business logic in `server/services/` if needed

## Security Considerations

- Always use HTTPS in production (TLS termination at reverse proxy)
- Change `JWT_SECRET` to a secure random string
- Configure OIDC provider with proper redirect URIs
- Review and limit `ADMIN_EMAILS` to trusted administrators
- Enable rate limiting on voting endpoints to prevent abuse
- Consider implementing machine authentication for Santa sync endpoints

## Deployment

### Production Build
```bash
# Build frontend and backend
make all

# Or manually:
cd client && npm install && npm run build
cp -r dist/* ../server/static/
cd .. && go build -ldflags="-s -w" -o krampus-server ./server

# Run with production config
export JWT_SECRET=$(openssl rand -base64 32)
export OIDC_PROVIDER_URL=https://your-oidc-provider.com
export OIDC_CLIENT_ID=your-client-id
export OIDC_CLIENT_SECRET=your-client-secret
export OIDC_REDIRECT_URL=https://your-domain.com/auth/callback
export SYNC_BASE_URL=https://your-domain.com
export ADMIN_EMAILS=admin@your-domain.com
export GIN_MODE=release

./krampus-server
```

### Reverse Proxy (Nginx)
```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Troubleshooting

### OIDC Authentication Fails
- Verify `OIDC_PROVIDER_URL` is correct and accessible
- Check client ID and secret are correct
- Ensure redirect URI is registered with the OIDC provider
- For Authentik: Ensure signing algorithm (HS256/RS256) is configured
- Check server logs for detailed error messages

### Database Errors
- Ensure `database/` directory exists and is writable
- Check `DATABASE_PATH` points to correct location
- Verify SQLite is installed (included in Go sqlite3 driver)
- Database migrations run automatically on startup

### Santa Client Can't Sync
- Verify `SYNC_BASE_URL` is accessible from client machines
- Check Santa plist configuration is correct
- Review server logs for sync errors
- Ensure machine is registered in the database

### Frontend Not Loading
- Verify frontend was built: `ls server/static/index.html`
- Rebuild if needed: `make frontend` or `cd client && npm run build`
- Check embedded assets loaded successfully in server logs
- Clear browser cache and reload

### User Not Admin
- Check `ADMIN_EMAILS` in `.env` contains user's email
- User must log in AFTER email is added to `ADMIN_EMAILS`
- For existing users, manually update database:
  ```bash
  sqlite3 database/krampus.db "UPDATE users SET role = 'ADMIN' WHERE email = 'user@example.com'"
  ```

## License

This project is provided as-is for use with NorthPole Security Santa (formerly Google Santa).

## Building and Development

### Build Commands (via Makefile)
- `make all` - Build frontend and backend
- `make frontend` - Build frontend only
- `make backend` - Build backend only
- `make clean` - Clean build artifacts
- `make run` - Build and run server

### Development Workflow
1. Make changes to frontend: `cd client && npm run dev` (dev server on port 5173)
2. Make changes to backend: Rebuild with `go build ./server`
3. Production build: `make all` to embed frontend in binary

## Contributing

Contributions are welcome! Please ensure:
- Code follows existing patterns
- Database migrations are included for schema changes
- API endpoints include proper authentication and validation
- Frontend components follow Material-UI patterns
- Tests are added for new features

## Support

For issues and questions:
- Check the [Santa documentation](https://santa.dev/) or [NorthPole Security Santa](https://github.com/northpolesec/santa)
- Review server logs for error messages
- Verify configuration in `.env` file
