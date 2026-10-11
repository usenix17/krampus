import { useMemo } from 'react';
import { Box, Button, Chip, Stack, Typography } from '@mui/material';
import { Download as DownloadIcon } from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/ToastContext';
import usePagedQuery from '../hooks/usePagedQuery';
import PageHeader from '../components/PageHeader';
import DataTable from '../components/DataTable';

const Programs = () => {
  const { isAdmin } = useAuth();
  const toast = useToast();
  const {
    items: programs,
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
  } = usePagedQuery('/api/programs');

  const handleAllowlist = async (identifier, ruleType, comment) => {
    try {
      await apiClient.post('/api/rules', {
        identifier,
        rule_type: ruleType,
        policy: 'ALLOWLIST',
        comment: comment || '',
      });
      toast.success(`Allowlisted ${ruleType} ${identifier.substring(0, 20)}...`);
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to allowlist');
    }
  };

  // Mirror the live search onto the CSV export link.
  const exportHref = useMemo(() => {
    const params = new URLSearchParams();
    if (search) params.set('search', search);
    const qs = params.toString();
    return `/api/programs/export${qs ? `?${qs}` : ''}`;
  }, [search]);

  const columns = [
    {
      key: 'program',
      label: 'Program',
      render: (row) => (
        <>
          <Typography variant="body2">{row.file_path || 'Unknown'}</Typography>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontFamily: 'monospace' }}
          >
            {row.file_hash ? `${row.file_hash.substring(0, 16)}...` : ''}
          </Typography>
        </>
      ),
    },
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
      key: 'certificate',
      label: 'Certificate',
      render: (row) => (
        <>
          {row.cert_cn && <Typography variant="body2">{row.cert_cn}</Typography>}
          {row.team_id && (
            <Typography variant="caption" color="text.secondary">
              Team: {row.team_id}
            </Typography>
          )}
        </>
      ),
    },
    { key: 'execution_count', label: 'Executions' },
    {
      key: 'status',
      label: 'Status',
      render: (row) => (
        <Box sx={{ display: 'flex', gap: 1 }}>
          {row.allow_count > 0 && (
            <Chip
              label={`${row.allow_count} Allowed`}
              color="success"
              size="small"
              variant="outlined"
            />
          )}
          {row.block_count > 0 && (
            <Chip
              label={`${row.block_count} Blocked`}
              color="error"
              size="small"
              variant="outlined"
            />
          )}
        </Box>
      ),
    },
    {
      key: 'last_seen',
      label: 'Last Seen',
      render: (row) => new Date(row.last_seen).toLocaleString(),
    },
  ];

  if (isAdmin()) {
    columns.push({
      key: 'allowlist',
      label: 'Allowlist',
      render: (row) => (
        <Stack direction="row" spacing={1}>
          {row.team_id && (
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
          {row.cert_sha256 && (
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
    });
  }

  return (
    <Box>
      <PageHeader title="Installed Programs & Binaries" />

      <DataTable
        columns={columns}
        rows={programs}
        total={total}
        page={page}
        perPage={perPage}
        onPageChange={setPage}
        onPerPageChange={setPerPage}
        loading={loading}
        error={error}
        onRetry={refresh}
        emptyMessage="No program execution data yet"
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Search programs..."
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

export default Programs;
