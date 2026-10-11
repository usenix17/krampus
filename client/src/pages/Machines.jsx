import { useCallback, useEffect, useState } from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
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
  Chip,
} from '@mui/material';
import { Add as AddIcon, Refresh as RefreshIcon, Download as DownloadIcon, Delete as DeleteIcon } from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/ToastContext';
import PageHeader from '../components/PageHeader';

const Machines = () => {
  const { isAdmin } = useAuth();
  const toast = useToast();
  const [machines, setMachines] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [openRegister, setOpenRegister] = useState(false);
  const [openMobileConfig, setOpenMobileConfig] = useState(false);
  const [selectedMachine, setSelectedMachine] = useState(null);
  const [registerData, setRegisterData] = useState({ machine_id: '', serial_number: '' });
  const [mobileConfigData, setMobileConfigData] = useState({
    client_mode: 'MONITOR',
    upload_interval: 600,
    organization_name: '',
    machine_owner: '',
  });

  // The machines endpoint returns a bare array (not paginated).
  const fetchMachines = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await apiClient.get('/api/machines');
      setMachines(Array.isArray(response.data) ? response.data : []);
    } catch (err) {
      setError(err.response?.data?.error || 'Failed to fetch machines');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchMachines();
  }, [fetchMachines]);

  const handleRegister = async () => {
    try {
      await apiClient.post('/api/machines', registerData);
      toast.success('Machine registered successfully!');
      setOpenRegister(false);
      setRegisterData({ machine_id: '', serial_number: '' });
      fetchMachines();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to register machine');
    }
  };

  const handleGenerateMobileConfig = async () => {
    try {
      const response = await apiClient.post(
        `/api/machines/${selectedMachine.machine_id}/mobileconfig`,
        mobileConfigData,
        { responseType: 'blob' }
      );

      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `${selectedMachine.machine_id}.mobileconfig`);
      document.body.appendChild(link);
      link.click();
      link.remove();

      toast.success('Mobileconfig downloaded successfully!');
      setOpenMobileConfig(false);
    } catch {
      toast.error('Failed to generate mobileconfig');
    }
  };

  const handleDelete = async (machineId) => {
    if (!window.confirm('Are you sure you want to delete this machine?')) return;
    try {
      await apiClient.delete(`/api/machines/${machineId}`);
      toast.success('Machine deleted successfully!');
      fetchMachines();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to delete machine');
    }
  };

  // Clearing the number field yields NaN; keep the previous value in that case.
  const handleUploadIntervalChange = (value) => {
    const parsed = parseInt(value, 10);
    setMobileConfigData((prev) => ({
      ...prev,
      upload_interval: Number.isNaN(parsed) ? prev.upload_interval : parsed,
    }));
  };

  return (
    <Box>
      <PageHeader
        title="Machines"
        actions={
          <>
            <IconButton aria-label="refresh machines" onClick={fetchMachines} sx={{ mr: 1 }}>
              <RefreshIcon />
            </IconButton>
            <Button variant="contained" startIcon={<AddIcon />} onClick={() => setOpenRegister(true)}>
              Register Machine
            </Button>
          </>
        }
      />

      {error && (
        <Alert
          severity="error"
          sx={{ mb: 2 }}
          action={
            <Button color="inherit" size="small" onClick={fetchMachines}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      )}

      {loading ? (
        <LinearProgress />
      ) : machines.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
          No machines found
        </Typography>
      ) : (
        <Grid container spacing={2}>
          {machines.map((machine) => (
            <Grid size={{ xs: 12, md: 6 }} key={machine.id}>
              <Card>
                <CardContent>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
                    <Typography variant="h6">{machine.machine_id}</Typography>
                    {isAdmin() && (
                      <IconButton
                        aria-label="delete machine"
                        onClick={() => handleDelete(machine.id)}
                        color="error"
                        size="small"
                      >
                        <DeleteIcon />
                      </IconButton>
                    )}
                  </Box>
                  <Box sx={{ mt: 1 }}>
                    {machine.client_mode && (
                      <Chip
                        label={machine.client_mode}
                        size="small"
                        color={machine.client_mode === 'LOCKDOWN' ? 'error' : 'warning'}
                        sx={{ mr: 1 }}
                      />
                    )}
                    {machine.hostname && (
                      <Typography variant="body2" color="text.secondary">
                        Hostname: {machine.hostname}
                      </Typography>
                    )}
                    {machine.serial_number && (
                      <Typography variant="body2" color="text.secondary">
                        Serial: {machine.serial_number}
                      </Typography>
                    )}
                    {machine.santa_version && (
                      <Typography variant="body2" color="text.secondary">
                        Santa: {machine.santa_version}
                      </Typography>
                    )}
                    {machine.last_sync && (
                      <Typography variant="caption" color="text.secondary">
                        Last sync: {new Date(machine.last_sync).toLocaleString()}
                      </Typography>
                    )}
                  </Box>
                  <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
                    <Button
                      variant="contained"
                      size="small"
                      startIcon={<DownloadIcon />}
                      onClick={() => {
                        setSelectedMachine(machine);
                        setOpenMobileConfig(true);
                      }}
                    >
                      Download Config
                    </Button>
                  </Box>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}

      <Dialog open={openRegister} onClose={() => setOpenRegister(false)}>
        <DialogTitle>Register New Machine</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            label="Machine ID"
            value={registerData.machine_id}
            onChange={(e) => setRegisterData({ ...registerData, machine_id: e.target.value })}
            margin="normal"
          />
          <TextField
            fullWidth
            label="Serial Number (optional)"
            value={registerData.serial_number}
            onChange={(e) => setRegisterData({ ...registerData, serial_number: e.target.value })}
            margin="normal"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenRegister(false)}>Cancel</Button>
          <Button onClick={handleRegister} variant="contained">
            Register
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={openMobileConfig} onClose={() => setOpenMobileConfig(false)}>
        <DialogTitle>Generate Mobileconfig Profile</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Recommended: Use mobileconfig for NorthPole Security Santa (official deployment method)
          </Typography>
          <TextField
            fullWidth
            select
            label="Client Mode"
            value={mobileConfigData.client_mode}
            onChange={(e) => setMobileConfigData({ ...mobileConfigData, client_mode: e.target.value })}
            margin="normal"
          >
            <MenuItem value="MONITOR">Monitor (allows everything, logs events)</MenuItem>
            <MenuItem value="LOCKDOWN">Lockdown (blocks unapproved binaries)</MenuItem>
          </TextField>
          <TextField
            fullWidth
            type="number"
            label="Upload Interval (seconds)"
            value={mobileConfigData.upload_interval}
            onChange={(e) => handleUploadIntervalChange(e.target.value)}
            margin="normal"
          />
          <TextField
            fullWidth
            label="Machine Owner (email or username)"
            value={mobileConfigData.machine_owner}
            onChange={(e) => setMobileConfigData({ ...mobileConfigData, machine_owner: e.target.value })}
            margin="normal"
            helperText="Required by Santa for sync to work"
          />
          <TextField
            fullWidth
            label="Organization Name (optional)"
            value={mobileConfigData.organization_name}
            onChange={(e) => setMobileConfigData({ ...mobileConfigData, organization_name: e.target.value })}
            margin="normal"
            helperText="Your company or organization name"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenMobileConfig(false)}>Cancel</Button>
          <Button onClick={handleGenerateMobileConfig} variant="contained">
            Download Mobileconfig
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Machines;
