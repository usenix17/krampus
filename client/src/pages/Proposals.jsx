import { useEffect, useState } from 'react';
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
} from '@mui/material';
import {
  ThumbUp,
  ThumbDown,
  Add as AddIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';

const Proposals = () => {
  const { isAdmin } = useAuth();
  const location = useLocation();
  const [proposals, setProposals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [openDialog, setOpenDialog] = useState(false);
  const [formData, setFormData] = useState({
    identifier: '',
    rule_type: 'BINARY',
    proposed_policy: 'ALLOWLIST',
    custom_message: '',
  });
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [highlightHash, setHighlightHash] = useState('');

  const fetchProposals = async () => {
    try {
      const response = await apiClient.get('/api/proposals');
      setProposals(response.data);
    } catch (error) {
      console.error('Failed to fetch proposals:', error);
      setError('Failed to fetch proposals');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchProposals();

    // Check for hash and machine query parameters from Santa block notification
    const params = new URLSearchParams(location.search);
    const hash = params.get('hash');
    const machine = params.get('machine');
    const comment = params.get('comment');

    if (hash) {
      setHighlightHash(hash.toLowerCase());

      // Wait for proposals to load, then check if one exists
      const checkAndCreateProposal = async () => {
        try {
          const response = await apiClient.get('/api/proposals');
          const existingProposal = response.data.find(
            p => p.identifier.toLowerCase() === hash.toLowerCase()
          );

          if (!existingProposal) {
            // Detect identifier type
            // SHA256 hashes are 64 hex characters
            // Bundle IDs typically look like: com.company.app
            const isSHA256 = /^[a-fA-F0-9]{64}$/.test(hash);
            const isBundleID = /^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)+$/.test(hash);

            let ruleType = 'BINARY'; // default
            if (isBundleID && !isSHA256) {
              ruleType = 'SIGNINGID'; // Bundle ID
            }

            // No proposal exists - pre-fill the form and open dialog
            let customMessage = '';
            if (comment && machine) {
              customMessage = `${comment} (requested from ${machine})`;
            } else if (comment) {
              customMessage = comment;
            } else if (machine) {
              customMessage = `Requested from ${machine}`;
            }

            setFormData({
              identifier: hash,
              rule_type: ruleType,
              proposed_policy: 'ALLOWLIST',
              custom_message: customMessage,
            });
            setOpenDialog(true);
            setError('No proposal found for this binary. Please create one to request access.');
          } else {
            // Proposal exists - just highlight it
            setSuccess('Found existing proposal for this binary. You can vote below.');
            setTimeout(() => setSuccess(''), 5000);
          }
        } catch (error) {
          console.error('Failed to check for existing proposal:', error);
        }
      };

      checkAndCreateProposal();
    }
  }, [location]);

  const handleVote = async (proposalId, voteType) => {
    try {
      await apiClient.post(`/api/proposals/${proposalId}/vote`, { vote_type: voteType });
      setSuccess('Vote submitted successfully!');
      fetchProposals();
      setTimeout(() => setSuccess(''), 3000);
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to submit vote');
      setTimeout(() => setError(''), 5000);
    }
  };

  const handleApprove = async (proposalId, policy) => {
    try {
      await apiClient.post(`/api/proposals/${proposalId}/approve`, { policy });
      setSuccess('Proposal approved!');
      fetchProposals();
      setTimeout(() => setSuccess(''), 3000);
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to approve proposal');
      setTimeout(() => setError(''), 5000);
    }
  };

  const handleCreate = async () => {
    try {
      await apiClient.post('/api/proposals', formData);
      setSuccess('Proposal created successfully!');
      setOpenDialog(false);
      setFormData({
        identifier: '',
        rule_type: 'BINARY',
        proposed_policy: 'BLOCKLIST',
        custom_message: '',
      });
      fetchProposals();
      setTimeout(() => setSuccess(''), 3000);
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to create proposal');
      setTimeout(() => setError(''), 5000);
    }
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

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4">Proposals</Typography>
        <Box>
          <IconButton onClick={fetchProposals} sx={{ mr: 1 }}>
            <RefreshIcon />
          </IconButton>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setOpenDialog(true)}
          >
            New Proposal
          </Button>
        </Box>
      </Box>

      {success && <Alert severity="success" sx={{ mb: 2 }}>{success}</Alert>}
      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}

      {loading ? (
        <LinearProgress />
      ) : (
        <Grid container spacing={2}>
          {proposals.map((proposal) => (
            <Grid item xs={12} key={proposal.id}>
              <Card
                sx={{
                  border: proposal.identifier.toLowerCase() === highlightHash ? '2px solid #1976d2' : 'none',
                  backgroundColor: proposal.identifier.toLowerCase() === highlightHash ? '#e3f2fd' : 'inherit',
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
                        Created by {proposal.creator_username} on {new Date(proposal.created_at).toLocaleString()}
                      </Typography>
                    </Box>
                    <Box sx={{ ml: 2, minWidth: 200 }}>
                      <Typography variant="body2" gutterBottom>
                        Votes:
                      </Typography>
                      <Box sx={{ mb: 1 }}>
                        <Typography variant="caption">Allowlist: {proposal.allowlist_votes}</Typography>
                        <LinearProgress
                          variant="determinate"
                          value={(proposal.allowlist_votes / 3) * 100}
                          sx={{ height: 8, borderRadius: 1, backgroundColor: '#e0e0e0' }}
                          color="success"
                        />
                      </Box>
                      <Box>
                        <Typography variant="caption">Blocklist: {proposal.blocklist_votes}</Typography>
                        <LinearProgress
                          variant="determinate"
                          value={(proposal.blocklist_votes / 3) * 100}
                          sx={{ height: 8, borderRadius: 1, backgroundColor: '#e0e0e0' }}
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
                    </Box>
                  </Box>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}

      <Dialog open={openDialog} onClose={() => setOpenDialog(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Create New Proposal</DialogTitle>
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
    </Box>
  );
};

export default Proposals;
