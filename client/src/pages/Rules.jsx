import { useMemo, useState } from 'react';
import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
  MenuItem,
  IconButton,
} from '@mui/material';
import {
  Add as AddIcon,
  Refresh as RefreshIcon,
  Delete as DeleteIcon,
  Download as DownloadIcon,
} from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/ToastContext';
import usePagedQuery from '../hooks/usePagedQuery';
import PageHeader from '../components/PageHeader';
import DataTable from '../components/DataTable';
import { formatDateTime } from '../utils/datetime';

const truncate = (value) =>
  value && value.length > 32 ? `${value.slice(0, 32)}...` : value;

// Honor a configured API origin (VITE_API_URL) so export links work in
// split-origin dev setups; the default '' keeps same-origin behavior.
const apiBase = apiClient.defaults.baseURL || '';

const Rules = () => {
  const { isAdmin } = useAuth();
  const toast = useToast();
  const {
    items: rules,
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
  } = usePagedQuery('/api/rules');
  const [openDialog, setOpenDialog] = useState(false);
  const [formData, setFormData] = useState({
    identifier: '',
    rule_type: 'BINARY',
    policy: 'BLOCKLIST',
    custom_message: '',
    comment: '',
  });

  const handleCreate = async () => {
    try {
      await apiClient.post('/api/rules', formData);
      toast.success('Rule created successfully!');
      setOpenDialog(false);
      setFormData({ identifier: '', rule_type: 'BINARY', policy: 'BLOCKLIST', custom_message: '', comment: '' });
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to create rule');
    }
  };

  // Mirror the live search onto the CSV export link.
  const exportHref = useMemo(() => {
    const params = new URLSearchParams();
    if (search) params.set('search', search);
    const qs = params.toString();
    return `${apiBase}/api/rules/export${qs ? `?${qs}` : ''}`;
  }, [search]);

  const handleDelete = async (ruleId) => {
    if (!window.confirm('Are you sure you want to delete this rule?')) return;
    try {
      await apiClient.delete(`/api/rules/${ruleId}`);
      toast.success('Rule deleted successfully!');
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to delete rule');
    }
  };

  const columns = [
    {
      key: 'identifier',
      label: 'Identifier',
      render: (row) => (
        <Typography variant="body2" sx={{ fontFamily: 'monospace' }} title={row.identifier}>
          {truncate(row.identifier)}
        </Typography>
      ),
    },
    {
      key: 'rule_type',
      label: 'Type',
      render: (row) => <Chip label={row.rule_type} size="small" />,
    },
    {
      key: 'policy',
      label: 'Policy',
      render: (row) => (
        <Chip
          label={row.policy}
          size="small"
          color={row.policy === 'BLOCKLIST' ? 'error' : 'success'}
        />
      ),
    },
    {
      key: 'comment',
      label: 'Comment',
      render: (row) => row.comment || '-',
    },
    {
      key: 'custom_message',
      label: 'Block Message',
      // When the comment and block message are identical, avoid showing it twice.
      render: (row) =>
        row.custom_message && row.custom_message !== row.comment
          ? row.custom_message
          : '-',
    },
    {
      key: 'created_at',
      label: 'Created',
      render: (row) => formatDateTime(row.created_at),
    },
  ];

  if (isAdmin()) {
    columns.push({
      key: 'actions',
      label: 'Actions',
      align: 'right',
      render: (row) => (
        <IconButton
          aria-label="delete rule"
          color="error"
          size="small"
          onClick={() => handleDelete(row.id)}
        >
          <DeleteIcon />
        </IconButton>
      ),
    });
  }

  return (
    <Box>
      <PageHeader
        title="Active Rules"
        actions={
          <>
            <IconButton aria-label="refresh rules" onClick={refresh} sx={{ mr: 1 }}>
              <RefreshIcon />
            </IconButton>
            {isAdmin() && (
              <Button variant="contained" startIcon={<AddIcon />} onClick={() => setOpenDialog(true)}>
                New Rule
              </Button>
            )}
          </>
        }
      />

      <DataTable
        columns={columns}
        rows={rules}
        total={total}
        page={page}
        perPage={perPage}
        onPageChange={setPage}
        onPerPageChange={setPerPage}
        loading={loading}
        error={error}
        onRetry={refresh}
        emptyMessage="No rules found"
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Search rules..."
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

      <Dialog open={openDialog} onClose={() => setOpenDialog(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Create New Rule</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            label="Identifier (SHA256 hash, cert hash, etc.)"
            value={formData.identifier}
            onChange={(e) => setFormData({ ...formData, identifier: e.target.value })}
            margin="normal"
          />
          <TextField
            fullWidth
            select
            label="Rule Type"
            value={formData.rule_type}
            onChange={(e) => setFormData({ ...formData, rule_type: e.target.value })}
            margin="normal"
          >
            <MenuItem value="BINARY">Binary</MenuItem>
            <MenuItem value="CERTIFICATE">Certificate</MenuItem>
            <MenuItem value="SIGNINGID">Signing ID</MenuItem>
            <MenuItem value="TEAMID">Team ID</MenuItem>
            <MenuItem value="CDHASH">CD Hash</MenuItem>
          </TextField>
          <TextField
            fullWidth
            select
            label="Policy"
            value={formData.policy}
            onChange={(e) => setFormData({ ...formData, policy: e.target.value })}
            margin="normal"
          >
            <MenuItem value="ALLOWLIST">Allowlist</MenuItem>
            <MenuItem value="BLOCKLIST">Blocklist</MenuItem>
          </TextField>
          <TextField
            fullWidth
            label="Comment (optional)"
            value={formData.comment}
            onChange={(e) => setFormData({ ...formData, comment: e.target.value })}
            margin="normal"
            helperText="Internal note to identify what application this rule is for"
          />
          <TextField
            fullWidth
            label="Custom Message (optional)"
            value={formData.custom_message}
            onChange={(e) => setFormData({ ...formData, custom_message: e.target.value })}
            margin="normal"
            multiline
            rows={3}
            helperText="Message shown to users when a binary is blocked"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenDialog(false)}>Cancel</Button>
          <Button onClick={handleCreate} variant="contained">
            Create
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Rules;
