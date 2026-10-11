import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box,
  Button,
  Chip,
  IconButton,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import {
  AddCircle as AddCircleIcon,
  Download as DownloadIcon,
} from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/ToastContext';
import usePagedQuery from '../hooks/usePagedQuery';
import PageHeader from '../components/PageHeader';
import DataTable from '../components/DataTable';
import { formatDateTime } from '../utils/datetime';

// Real agents send prefixed decisions (ALLOW_BINARY, BLOCK_BINARY), so
// color by prefix rather than exact value.
// Honor a configured API origin (VITE_API_URL) so export links work in
// split-origin dev setups; the default '' keeps same-origin behavior.
const apiBase = apiClient.defaults.baseURL || '';

const getDecisionColor = (decision) => {
  if (decision?.startsWith('ALLOW')) return 'success';
  if (decision?.startsWith('BLOCK')) return 'error';
  return 'default';
};

const Events = () => {
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  const toast = useToast();
  const [machineId, setMachineId] = useState('');
  const [decision, setDecision] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [machines, setMachines] = useState([]);

  // Only send filters that are set; a stable object keyed on the raw values
  // keeps the hook from refetching on unrelated renders.
  const extraParams = useMemo(() => {
    const params = {};
    if (machineId) params.machine_id = machineId;
    if (decision) params.decision = decision;
    if (from) params.from = from;
    if (to) params.to = to;
    return params;
  }, [machineId, decision, from, to]);

  const {
    items: events,
    total,
    page,
    perPage,
    search,
    loading,
    error,
    setPage,
    setPerPage,
    setSearch,
    refresh,
  } = usePagedQuery('/api/events', extraParams);

  useEffect(() => {
    let active = true;
    apiClient
      .get('/api/machines')
      .then((response) => {
        if (active && Array.isArray(response.data)) setMachines(response.data);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, []);

  const handleAllowlist = async (identifier, ruleType, comment) => {
    try {
      await apiClient.post('/api/rules', {
        identifier,
        rule_type: ruleType,
        policy: 'ALLOWLIST',
        comment: comment || '',
      });
      toast.success(`Allowlisted ${ruleType}`);
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to allowlist');
    }
  };

  const handleCreateRule = (event) => {
    // Carry every identifier the event holds so the proposal form can switch
    // rule type and auto-fill the right one.
    const params = new URLSearchParams({ hash: event.file_hash });
    if (event.cert_sha256) params.append('cert_sha256', event.cert_sha256);
    if (event.signing_id) params.append('signing_id', event.signing_id);
    if (event.team_id) params.append('team_id', event.team_id);
    if (event.bundle_name) {
      params.append('comment', event.bundle_name);
    } else if (event.bundle_id) {
      params.append('comment', event.bundle_id);
    }
    navigate(`/proposals?${params.toString()}`);
  };

  const clearFilters = () => {
    setMachineId('');
    setDecision('');
    setFrom('');
    setTo('');
  };

  // Mirror the live filters and search onto the CSV export link.
  const exportHref = useMemo(() => {
    const params = new URLSearchParams();
    if (search) params.set('search', search);
    if (machineId) params.set('machine_id', machineId);
    if (decision) params.set('decision', decision);
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    const qs = params.toString();
    return `${apiBase}/api/events/export${qs ? `?${qs}` : ''}`;
  }, [search, machineId, decision, from, to]);

  const columns = [
    {
      key: 'execution_time',
      label: 'Timestamp',
      render: (row) => formatDateTime(row.execution_time),
    },
    { key: 'machine_id', label: 'Machine' },
    {
      key: 'file_path',
      label: 'File Path',
      render: (row) => (
        <Typography
          variant="body2"
          sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
        >
          {row.file_path}
        </Typography>
      ),
    },
    {
      key: 'file_hash',
      label: 'SHA256 Hash',
      render: (row) => (
        <Typography
          variant="body2"
          sx={{
            fontFamily: 'monospace',
            fontSize: '0.7rem',
            wordBreak: 'break-all',
            maxWidth: '400px',
          }}
        >
          {row.file_hash}
        </Typography>
      ),
    },
    {
      key: 'decision',
      label: 'Decision',
      render: (row) => (
        <Chip
          label={row.decision}
          color={getDecisionColor(row.decision)}
          size="small"
        />
      ),
    },
    { key: 'executing_user', label: 'User' },
    {
      key: 'bundle',
      label: 'Bundle',
      render: (row) =>
        row.bundle_name ? (
          <>
            <Typography variant="body2">{row.bundle_name}</Typography>
            <Typography variant="caption" color="text.secondary">
              {row.bundle_id}
            </Typography>
          </>
        ) : null,
    },
    {
      key: 'actions',
      label: 'Actions',
      render: (row) => (
        <Stack direction="row" spacing={1} alignItems="center">
          <Tooltip title="Propose a rule for this binary">
            <IconButton
              size="small"
              color="primary"
              aria-label="propose rule"
              onClick={() => handleCreateRule(row)}
            >
              <AddCircleIcon />
            </IconButton>
          </Tooltip>
          {isAdmin() && row.team_id && (
            <Button
              size="small"
              variant="outlined"
              color="success"
              onClick={() =>
                handleAllowlist(
                  row.team_id,
                  'TEAMID',
                  row.bundle_name || row.file_path
                )
              }
            >
              Team ID
            </Button>
          )}
          {isAdmin() && row.cert_sha256 && (
            <Button
              size="small"
              variant="outlined"
              color="success"
              onClick={() =>
                handleAllowlist(
                  row.cert_sha256,
                  'CERTIFICATE',
                  row.cert_cn || row.bundle_name
                )
              }
            >
              Cert
            </Button>
          )}
        </Stack>
      ),
    },
  ];

  return (
    <Box>
      <PageHeader title="Execution Events" />

      <Stack
        direction="row"
        spacing={2}
        sx={{ mb: 2, flexWrap: 'wrap', gap: 2 }}
        alignItems="center"
      >
        <TextField
          select
          label="Decision"
          value={decision}
          onChange={(e) => setDecision(e.target.value)}
          size="small"
          sx={{ minWidth: 150 }}
        >
          <MenuItem value="">All</MenuItem>
          <MenuItem value="ALLOW">Allow</MenuItem>
          <MenuItem value="BLOCK">Block</MenuItem>
        </TextField>
        <TextField
          select
          label="Machine"
          value={machineId}
          onChange={(e) => setMachineId(e.target.value)}
          size="small"
          sx={{ minWidth: 200 }}
        >
          <MenuItem value="">All</MenuItem>
          {machines.map((m) => (
            <MenuItem key={m.machine_id} value={m.machine_id}>
              {m.hostname || m.machine_id}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          label="From"
          type="date"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
          size="small"
          slotProps={{ inputLabel: { shrink: true } }}
        />
        <TextField
          label="To"
          type="date"
          value={to}
          onChange={(e) => setTo(e.target.value)}
          size="small"
          slotProps={{ inputLabel: { shrink: true } }}
        />
        <Button onClick={clearFilters}>Clear filters</Button>
      </Stack>

      <DataTable
        columns={columns}
        rows={events}
        total={total}
        page={page}
        perPage={perPage}
        onPageChange={setPage}
        onPerPageChange={setPerPage}
        loading={loading}
        error={error}
        onRetry={refresh}
        emptyMessage="No events found"
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Search events..."
        toolbarActions={
          <Button
            component="a"
            href={exportHref}
            download
            startIcon={<DownloadIcon />}
            variant="outlined"
          >
            Export CSV
          </Button>
        }
      />
    </Box>
  );
};

export default Events;
