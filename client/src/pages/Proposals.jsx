import { useEffect, useMemo, useState } from 'react';
import { useLocation } from 'react-router-dom';
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Grid,
  LinearProgress,
  TextField,
  Typography,
  MenuItem,
  Alert,
  IconButton,
  InputAdornment,
  List,
  ListItem,
  ListItemText,
  TablePagination,
  Skeleton,
} from '@mui/material';
import {
  ThumbUp,
  ThumbDown,
  Add as AddIcon,
  Refresh as RefreshIcon,
  Search as SearchIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import { useTheme } from '@mui/material/styles';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/ToastContext';
import usePagedQuery from '../hooks/usePagedQuery';
import PageHeader from '../components/PageHeader';
import { formatDateTime } from '../utils/datetime';

const Proposals = () => {
  const { isAdmin } = useAuth();
  const toast = useToast();
  const theme = useTheme();
  const location = useLocation();
  const {
    items: proposals,
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
  } = usePagedQuery('/api/proposals');
  const [openDialog, setOpenDialog] = useState(false);
  const [formData, setFormData] = useState({
    identifier: '',
    rule_type: 'BINARY',
    proposed_policy: 'ALLOWLIST',
    custom_message: '',
  });
  const [voteThreshold, setVoteThreshold] = useState(3); // Default to 3
  const [votesDialog, setVotesDialog] = useState({ open: false, loading: false, votes: [] });
  // Identifiers available for the binary a proposal is being created from,
  // keyed by rule type. Lets the Rule Type dropdown auto-fill the identifier.
  const [identifierOptions, setIdentifierOptions] = useState({});

  // Hash from the Santa block notification used to highlight a matching card.
  const highlightHash = useMemo(
    () => (new URLSearchParams(location.search).get('hash') || '').toLowerCase(),
    [location.search]
  );

  useEffect(() => {
    const fetchConfig = async () => {
      try {
        const response = await apiClient.get('/api/config');
        setVoteThreshold(response.data.vote_threshold || 3);
      } catch (err) {
        console.error('Failed to fetch config:', err);
      }
    };
    fetchConfig();
  }, []);

  useEffect(() => {
    // Check for hash and machine query parameters from Santa block notification
    const params = new URLSearchParams(location.search);
    const hash = params.get('hash');
    const machine = params.get('machine');
    const comment = params.get('comment');
    const certSha256 = params.get('cert_sha256');
    const signingId = params.get('signing_id');
    const teamId = params.get('team_id');

    if (!hash) return;

    const checkAndCreateProposal = async () => {
      try {
        const response = await apiClient.get('/api/proposals', {
          params: { search: hash, per_page: 100 },
        });
        const list = response.data?.items || [];
        const existingProposal = list.find(
          (p) => p.identifier.toLowerCase() === hash.toLowerCase()
        );

        if (!existingProposal) {
          const isSHA256 = /^[a-fA-F0-9]{64}$/.test(hash);
          const isBundleID = /^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)+$/.test(hash);

          let ruleType = 'BINARY';
          const idOptions = {};
          if (isBundleID && !isSHA256) {
            ruleType = 'SIGNINGID';
            idOptions.SIGNINGID = hash;
          } else {
            idOptions.BINARY = hash;
          }
          if (certSha256) idOptions.CERTIFICATE = certSha256;
          if (signingId) idOptions.SIGNINGID = signingId;
          if (teamId) idOptions.TEAMID = teamId;
          setIdentifierOptions(idOptions);

          let customMessage = '';
          if (comment && machine) {
            customMessage = `${comment} (requested from ${machine})`;
          } else if (comment) {
            customMessage = comment;
          } else if (machine) {
            customMessage = `Requested from ${machine}`;
          }

          setFormData({
            identifier: idOptions[ruleType] || hash,
            rule_type: ruleType,
            proposed_policy: 'ALLOWLIST',
            custom_message: customMessage,
          });
          setOpenDialog(true);
          toast.error('No proposal found for this binary. Please create one to request access.');
        } else {
          toast.success('Found existing proposal for this binary. You can vote below.');
        }
      } catch (err) {
        console.error('Failed to check for existing proposal:', err);
      }
    };

    checkAndCreateProposal();
  }, [location, toast]);

  const handleVote = async (proposalId, voteType) => {
    try {
      await apiClient.post(`/api/proposals/${proposalId}/vote`, { vote_type: voteType });
      toast.success('Vote submitted successfully!');
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to submit vote');
    }
  };

  const handleApprove = async (proposalId, policy) => {
    try {
      await apiClient.post(`/api/proposals/${proposalId}/approve`, { policy });
      toast.success('Proposal approved!');
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to approve proposal');
    }
  };

  // Load vote history on demand so cards stay cheap until the user asks.
  const handleViewVotes = async (proposalId) => {
    setVotesDialog({ open: true, loading: true, votes: [] });
    try {
      const response = await apiClient.get(`/api/proposals/${proposalId}/votes`);
      setVotesDialog({ open: true, loading: false, votes: response.data?.votes || [] });
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to fetch votes');
      setVotesDialog({ open: false, loading: false, votes: [] });
    }
  };

  const handleReject = async (proposalId) => {
    if (!window.confirm('Are you sure you want to reject this proposal?')) return;
    try {
      await apiClient.post(`/api/proposals/${proposalId}/reject`);
      toast.success('Proposal rejected');
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to reject proposal');
    }
  };

  const handleDelete = async (proposalId) => {
    if (!window.confirm('Are you sure you want to delete this proposal?')) return;
    try {
      await apiClient.delete(`/api/proposals/${proposalId}`);
      toast.success('Proposal deleted successfully!');
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to delete proposal');
    }
  };

  const handleCreate = async () => {
    try {
      await apiClient.post('/api/proposals', formData);
      toast.success('Proposal created successfully!');
      setOpenDialog(false);
      setIdentifierOptions({});
      setFormData({
        identifier: '',
        rule_type: 'BINARY',
        proposed_policy: 'ALLOWLIST',
        custom_message: '',
      });
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to create proposal');
    }
  };

  // When the rule type changes and we came from a specific binary, swap the
  // identifier to the one that matches the newly selected type.
  const handleRuleTypeChange = (newType) => {
    setFormData((prev) => {
      const hasOptions = Object.keys(identifierOptions).length > 0;
      return {
        ...prev,
        rule_type: newType,
        identifier: hasOptions ? (identifierOptions[newType] || '') : prev.identifier,
      };
    });
  };

  const openBlankProposal = () => {
    setIdentifierOptions({});
    setFormData({
      identifier: '',
      rule_type: 'BINARY',
      proposed_policy: 'ALLOWLIST',
      custom_message: '',
    });
    setOpenDialog(true);
  };

  const getStatusColor = (status) => {
    switch (status) {
      case 'PENDING':
        return 'warning';
      case 'APPROVED':
        return 'success';
      case 'REJECTED':
        return 'error';
      default:
        return 'default';
    }
  };

  const progress = (votes) => Math.min(100, (votes / Math.max(1, voteThreshold)) * 100);

  return (
    <Box>
      <PageHeader
        title="Proposals"
        actions={
          <>
            <IconButton aria-label="refresh proposals" onClick={refresh} sx={{ mr: 1 }}>
              <RefreshIcon />
            </IconButton>
            <Button variant="contained" startIcon={<AddIcon />} onClick={openBlankProposal}>
              New Proposal
            </Button>
          </>
        }
      />

      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2, flexWrap: 'wrap' }}>
        <TextField
          size="small"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search proposals..."
          slotProps={{
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon fontSize="small" />
                </InputAdornment>
              ),
            },
          }}
          sx={{ minWidth: 240 }}
        />
      </Box>

      {error && (
        <Alert
          severity="error"
          sx={{ mb: 2 }}
          action={
            <Button color="inherit" size="small" onClick={refresh}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      )}

      {loading ? (
        <Grid container spacing={2}>
          {[0, 1, 2].map((i) => (
            <Grid size={{ xs: 12 }} key={`skeleton-${i}`}>
              <Skeleton variant="rounded" height={140} />
            </Grid>
          ))}
        </Grid>
      ) : proposals.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
          No proposals found
        </Typography>
      ) : (
        <Grid container spacing={2}>
          {proposals.map((proposal) => {
            const highlighted = proposal.identifier.toLowerCase() === highlightHash;
            return (
              <Grid size={{ xs: 12 }} key={proposal.id}>
                <Card
                  sx={{
                    border: highlighted ? `2px solid ${theme.palette.primary.main}` : 'none',
                    backgroundColor: highlighted ? theme.palette.action.hover : 'inherit',
                  }}
                >
                  <CardContent>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start', mb: 2 }}>
                      <Box sx={{ flex: 1 }}>
                        <Typography variant="h6" sx={{ wordBreak: 'break-all' }}>
                          {proposal.identifier}
                        </Typography>
                        <Box sx={{ mt: 1 }}>
                          <Chip label={proposal.rule_type} size="small" sx={{ mr: 1 }} />
                          <Chip
                            label={proposal.proposed_policy}
                            size="small"
                            color={proposal.proposed_policy === 'BLOCKLIST' ? 'error' : 'success'}
                            sx={{ mr: 1 }}
                          />
                          <Chip label={proposal.status} size="small" color={getStatusColor(proposal.status)} />
                        </Box>
                        {proposal.custom_message && (
                          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                            {proposal.custom_message}
                          </Typography>
                        )}
                        <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                          Created by {proposal.creator_username} on {formatDateTime(proposal.created_at)}
                        </Typography>
                        <Button size="small" sx={{ mt: 1, px: 0 }} onClick={() => handleViewVotes(proposal.id)}>
                          View votes
                        </Button>
                      </Box>
                      <Box sx={{ ml: 2, minWidth: 200 }}>
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                          <Typography variant="body2" gutterBottom>
                            Votes:
                          </Typography>
                          {isAdmin() && (
                            <IconButton
                              aria-label="delete proposal"
                              color="error"
                              size="small"
                              onClick={() => handleDelete(proposal.id)}
                            >
                              <DeleteIcon fontSize="small" />
                            </IconButton>
                          )}
                        </Box>
                        <Box sx={{ mb: 1 }}>
                          <Typography variant="caption">Allowlist: {proposal.allowlist_votes}/{voteThreshold}</Typography>
                          <LinearProgress
                            variant="determinate"
                            value={progress(proposal.allowlist_votes)}
                            sx={{ height: 8, borderRadius: 1 }}
                            color="success"
                          />
                        </Box>
                        <Box>
                          <Typography variant="caption">Blocklist: {proposal.blocklist_votes}/{voteThreshold}</Typography>
                          <LinearProgress
                            variant="determinate"
                            value={progress(proposal.blocklist_votes)}
                            sx={{ height: 8, borderRadius: 1 }}
                            color="error"
                          />
                        </Box>
                        {proposal.status === 'PENDING' && (
                          <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
                            <Button
                              size="small"
                              variant="outlined"
                              color="success"
                              startIcon={<ThumbUp />}
                              onClick={() => handleVote(proposal.id, 'ALLOWLIST')}
                            >
                              Allowlist
                            </Button>
                            <Button
                              size="small"
                              variant="outlined"
                              color="error"
                              startIcon={<ThumbDown />}
                              onClick={() => handleVote(proposal.id, 'BLOCKLIST')}
                            >
                              Blocklist
                            </Button>
                          </Box>
                        )}
                        {isAdmin() && proposal.status === 'PENDING' && (
                          <Box sx={{ mt: 1, display: 'flex', gap: 1 }}>
                            <Button
                              size="small"
                              variant="contained"
                              color="success"
                              onClick={() => handleApprove(proposal.id, 'ALLOWLIST')}
                            >
                              Approve Allow
                            </Button>
                            <Button
                              size="small"
                              variant="contained"
                              color="error"
                              onClick={() => handleApprove(proposal.id, 'BLOCKLIST')}
                            >
                              Approve Block
                            </Button>
                          </Box>
                        )}
                        {isAdmin() && proposal.status === 'PENDING' && (
                          <Box sx={{ mt: 1 }}>
                            <Button
                              size="small"
                              variant="outlined"
                              color="inherit"
                              onClick={() => handleReject(proposal.id)}
                            >
                              Reject
                            </Button>
                          </Box>
                        )}
                      </Box>
                    </Box>
                  </CardContent>
                </Card>
              </Grid>
            );
          })}
        </Grid>
      )}

      <TablePagination
        component="div"
        count={total}
        page={total > 0 ? page - 1 : 0}
        rowsPerPage={perPage}
        onPageChange={(event, newPage) => setPage(newPage + 1)}
        onRowsPerPageChange={(event) => setPerPage(parseInt(event.target.value, 10))}
        rowsPerPageOptions={[10, 25, 50, 100]}
      />

      <Dialog open={openDialog} onClose={() => setOpenDialog(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Create New Proposal</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            select
            label="Rule Type"
            value={formData.rule_type}
            onChange={(e) => handleRuleTypeChange(e.target.value)}
            margin="normal"
            helperText={
              Object.keys(identifierOptions).length > 0
                ? 'Identifier auto-fills to match the selected rule type'
                : ''
            }
          >
            <MenuItem value="BINARY">Binary</MenuItem>
            <MenuItem value="CERTIFICATE">Certificate</MenuItem>
            <MenuItem value="SIGNINGID">Signing ID</MenuItem>
            <MenuItem value="TEAMID">Team ID</MenuItem>
            <MenuItem value="CDHASH">CD Hash</MenuItem>
          </TextField>
          <TextField
            fullWidth
            label="Identifier (SHA256 hash, cert hash, etc.)"
            value={formData.identifier}
            onChange={(e) => setFormData({ ...formData, identifier: e.target.value })}
            margin="normal"
            helperText={
              Object.keys(identifierOptions).length > 0 && !identifierOptions[formData.rule_type]
                ? 'This event has no identifier for the selected type -- enter one manually'
                : ''
            }
          />
          <TextField
            fullWidth
            select
            label="Proposed Policy"
            value={formData.proposed_policy}
            onChange={(e) => setFormData({ ...formData, proposed_policy: e.target.value })}
            margin="normal"
          >
            <MenuItem value="ALLOWLIST">Allowlist</MenuItem>
            <MenuItem value="BLOCKLIST">Blocklist</MenuItem>
          </TextField>
          <TextField
            fullWidth
            label="Custom Message (optional)"
            value={formData.custom_message}
            onChange={(e) => setFormData({ ...formData, custom_message: e.target.value })}
            margin="normal"
            multiline
            rows={3}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenDialog(false)}>Cancel</Button>
          <Button onClick={handleCreate} variant="contained">
            Create
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={votesDialog.open}
        onClose={() => setVotesDialog({ open: false, loading: false, votes: [] })}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>Vote History</DialogTitle>
        <DialogContent>
          {votesDialog.loading ? (
            <Skeleton variant="rounded" height={120} />
          ) : votesDialog.votes.length === 0 ? (
            <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
              No votes yet
            </Typography>
          ) : (
            <List dense>
              {votesDialog.votes.map((vote, index) => (
                <ListItem key={`${vote.username}-${index}`} disableGutters>
                  <ListItemText
                    primary={vote.username}
                    secondary={formatDateTime(vote.created_at)}
                  />
                  <Chip
                    label={vote.vote_type}
                    size="small"
                    color={vote.vote_type === 'BLOCKLIST' ? 'error' : 'success'}
                  />
                </ListItem>
              ))}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setVotesDialog({ open: false, loading: false, votes: [] })}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Proposals;
