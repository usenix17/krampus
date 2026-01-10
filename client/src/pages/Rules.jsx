import { useEffect, useState } from 'react';
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
import { Add as AddIcon, Refresh as RefreshIcon, Delete as DeleteIcon } from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';

const Rules = () => {
  const { isAdmin } = useAuth();
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [openDialog, setOpenDialog] = useState(false);
  const [formData, setFormData] = useState({
    identifier: '',
    rule_type: 'BINARY',
    policy: 'BLOCKLIST',
    custom_message: '',
    comment: '',
  });
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  const fetchRules = async () => {
    try {
      const response = await apiClient.get('/api/rules');
      setRules(response.data);
    } catch (error) {
      console.error('Failed to fetch rules:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRules();
  }, []);

  const handleCreate = async () => {
    try {
      await apiClient.post('/api/rules', formData);
      setSuccess('Rule created successfully!');
      setOpenDialog(false);
      setFormData({ identifier: '', rule_type: 'BINARY', policy: 'BLOCKLIST', custom_message: '', comment: '' });
      fetchRules();
      setTimeout(() => setSuccess(''), 3000);
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to create rule');
      setTimeout(() => setError(''), 5000);
    }
  };

  const handleDelete = async (ruleId) => {
    if (!window.confirm('Are you sure you want to delete this rule?')) return;
    try {
      await apiClient.delete(`/api/rules/${ruleId}`);
      setSuccess('Rule deleted successfully!');
      fetchRules();
      setTimeout(() => setSuccess(''), 3000);
    } catch (error) {
      setError(error.response?.data?.error || 'Failed to delete rule');
      setTimeout(() => setError(''), 5000);
    }
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4">Active Rules</Typography>
        <Box>
          <IconButton onClick={fetchRules} sx={{ mr: 1 }}>
            <RefreshIcon />
          </IconButton>
          {isAdmin() && (
            <Button variant="contained" startIcon={<AddIcon />} onClick={() => setOpenDialog(true)}>
              New Rule
            </Button>
          )}
        </Box>
      </Box>

      {success && <Alert severity="success" sx={{ mb: 2 }}>{success}</Alert>}
      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}

      {loading ? (
        <LinearProgress />
      ) : (
        <Grid container spacing={2}>
          {rules.map((rule) => (
            <Grid item xs={12} key={rule.id}>
              <Card>
                <CardContent>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
                    <Box sx={{ flex: 1 }}>
                      <Typography variant="h6" sx={{ wordBreak: 'break-all' }}>
                        {rule.identifier}
                      </Typography>
                      <Box sx={{ mt: 1 }}>
                        <Chip label={rule.rule_type} size="small" sx={{ mr: 1 }} />
                        <Chip
                          label={rule.policy}
                          size="small"
                          color={rule.policy === 'BLOCKLIST' ? 'error' : 'success'}
                        />
                      </Box>
                      {rule.comment && (
                        <Typography variant="body2" sx={{ mt: 1, fontWeight: 500 }}>
                          {rule.comment}
                        </Typography>
                      )}
                      {rule.custom_message && (
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                          Block message: {rule.custom_message}
                        </Typography>
                      )}
                      <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                        Created on {new Date(rule.created_at).toLocaleString()}
                      </Typography>
                    </Box>
                    {isAdmin() && (
                      <IconButton onClick={() => handleDelete(rule.id)} color="error">
                        <DeleteIcon />
                      </IconButton>
                    )}
                  </Box>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}

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
