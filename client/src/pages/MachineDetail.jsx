import { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Grid,
  Skeleton,
  Typography,
} from '@mui/material';
import {
  ArrowBack as ArrowBackIcon,
  Download as DownloadIcon,
} from '@mui/icons-material';
import apiClient from '../api/client';
import { useToast } from '../contexts/ToastContext';
import usePagedQuery from '../hooks/usePagedQuery';
import PageHeader from '../components/PageHeader';
import DataTable from '../components/DataTable';
import { formatDateTime } from '../utils/datetime';

// No /api/config value for staleness yet, so treat a machine with no sync in
// the last 24h as stale.
const STALE_AFTER_HOURS = 24;

const getDecisionColor = (decision) => {
  if (decision?.startsWith('ALLOW')) return 'success';
  if (decision?.startsWith('BLOCK')) return 'error';
  return 'default';
};

// Describe sync freshness relative to now; drives the header health chip.
const syncHealth = (lastSync) => {
  if (!lastSync) return { label: 'never synced', color: 'error' };
  const then = new Date(lastSync).getTime();
  if (Number.isNaN(then)) return { label: 'never synced', color: 'error' };
  const hours = Math.max(0, Math.round((Date.now() - then) / 3600000));
  if (hours >= STALE_AFTER_HOURS) {
    return { label: `no sync for ${hours}h`, color: 'warning' };
  }
  return { label: `synced ${hours}h ago`, color: 'success' };
};

const columns = [
  {
    key: 'execution_time',
    label: 'Timestamp',
    render: (row) => formatDateTime(row.execution_time),
  },
  {
    key: 'decision',
    label: 'Decision',
    render: (row) =>
      row.decision ? (
        <Chip label={row.decision} color={getDecisionColor(row.decision)} size="small" />
      ) : (
        '-'
      ),
  },
  {
    key: 'file_path',
    label: 'File Path',
    render: (row) => (
      <Typography
        variant="body2"
        sx={{ fontFamily: 'monospace', fontSize: '0.75rem', wordBreak: 'break-all', maxWidth: 360 }}
        title={row.file_path || ''}
      >
        {row.file_path || '-'}
      </Typography>
    ),
  },
  { key: 'bundle_name', label: 'Bundle', render: (row) => row.bundle_name || '-' },
  { key: 'executing_user', label: 'User', render: (row) => row.executing_user || '-' },
];

// One label/value pair in the machine info grid.
const InfoItem = ({ label, children }) => (
  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
    <Typography variant="caption" color="text.secondary" display="block">
      {label}
    </Typography>
    <Box sx={{ wordBreak: 'break-all' }}>{children ?? '-'}</Box>
  </Grid>
);

const MachineDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const [machine, setMachine] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notFound, setNotFound] = useState(false);

  const fetchMachine = useCallback(async () => {
    setLoading(true);
    setError('');
    setNotFound(false);
    try {
      const response = await apiClient.get(`/api/machines/${id}`);
      setMachine(response.data);
    } catch (err) {
      if (err.response?.status === 404) {
        setNotFound(true);
      } else {
        setError(err.response?.data?.error || 'Failed to fetch machine');
      }
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchMachine();
  }, [fetchMachine]);

  const events = usePagedQuery(`/api/machines/${id}/events`);

  const handleDownloadConfig = async () => {
    try {
      const response = await apiClient.post(
        `/api/machines/${machine.machine_id}/mobileconfig`,
        { client_mode: machine.client_mode || 'MONITOR', upload_interval: 600 },
        { responseType: 'blob' }
      );
      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `${machine.machine_id}.mobileconfig`);
      document.body.appendChild(link);
      link.click();
      link.remove();
      toast.success('Mobileconfig downloaded successfully!');
    } catch {
      toast.error('Failed to generate mobileconfig');
    }
  };

  const backButton = (
    <Button startIcon={<ArrowBackIcon />} onClick={() => navigate('/machines')}>
      Back to Machines
    </Button>
  );

  if (loading) {
    return (
      <Box>
        <PageHeader title="Machine" actions={backButton} />
        <Skeleton variant="rounded" height={220} />
      </Box>
    );
  }

  if (notFound) {
    return (
      <Box>
        <PageHeader title="Machine" actions={backButton} />
        <Alert severity="warning">Machine not found.</Alert>
      </Box>
    );
  }

  if (error) {
    return (
      <Box>
        <PageHeader title="Machine" actions={backButton} />
        <Alert
          severity="error"
          action={
            <Button color="inherit" size="small" onClick={fetchMachine}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      </Box>
    );
  }

  const health = syncHealth(machine.last_sync);
  const subtitleParts = [];
  if (machine.serial_number) subtitleParts.push(machine.serial_number);
  if (machine.os_version) subtitleParts.push(`macOS ${machine.os_version}`);

  return (
    <Box>
      <PageHeader
        title={machine.hostname || machine.machine_id}
        subtitle={subtitleParts.join(' · ') || undefined}
        actions={
          <Box sx={{ display: 'flex', gap: 1 }}>
            {backButton}
            <Button variant="contained" startIcon={<DownloadIcon />} onClick={handleDownloadConfig}>
              Download Config
            </Button>
          </Box>
        }
      />

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2, flexWrap: 'wrap' }}>
            <Chip label={health.label} color={health.color} size="small" />
            {machine.client_mode && (
              <Chip
                label={machine.client_mode}
                size="small"
                color={machine.client_mode === 'LOCKDOWN' ? 'error' : 'warning'}
              />
            )}
          </Box>
          <Grid container spacing={2}>
            <InfoItem label="Machine ID">{machine.machine_id}</InfoItem>
            <InfoItem label="Serial Number">{machine.serial_number}</InfoItem>
            <InfoItem label="OS Version">{machine.os_version}</InfoItem>
            <InfoItem label="OS Build">{machine.os_build}</InfoItem>
            <InfoItem label="Santa Version">{machine.santa_version}</InfoItem>
            <InfoItem label="Enrolled At">{formatDateTime(machine.enrolled_at)}</InfoItem>
            <InfoItem label="Last Sync">{formatDateTime(machine.last_sync)}</InfoItem>
            <InfoItem label="Last Preflight Sync">
              {formatDateTime(machine.last_preflight_sync)}
            </InfoItem>
          </Grid>
        </CardContent>
      </Card>

      <Typography variant="h6" sx={{ mb: 2 }}>
        Recent Events
      </Typography>
      <DataTable
        columns={columns}
        rows={events.items}
        total={events.total}
        page={events.page}
        perPage={events.perPage}
        onPageChange={events.setPage}
        onPerPageChange={events.setPerPage}
        loading={events.loading}
        error={events.error}
        onRetry={events.refresh}
        emptyMessage="No events for this machine"
      />
    </Box>
  );
};

export default MachineDetail;
