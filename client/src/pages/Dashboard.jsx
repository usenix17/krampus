import { useEffect, useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Grid,
  Typography,
  CircularProgress,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import {
  HowToVote as VoteIcon,
  Rule as RuleIcon,
  Computer as ComputerIcon,
  People as PeopleIcon,
} from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';

const StatCard = ({ title, value, icon, color, contrast }) => (
  <Card>
    <CardContent>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
        <Box
          sx={{
            backgroundColor: color,
            color: contrast,
            borderRadius: '50%',
            p: 1,
            mr: 2,
            display: 'flex',
          }}
        >
          {icon}
        </Box>
        <Typography variant="h6">{title}</Typography>
      </Box>
      <Typography variant="h3">{value !== null && value !== undefined ? value : '...'}</Typography>
    </CardContent>
  </Card>
);

const Dashboard = () => {
  const { user, isAdmin } = useAuth();
  const theme = useTheme();
  const { stats: statColors } = theme.palette;
  const [stats, setStats] = useState({
    proposals: null,
    rules: null,
    machines: null,
    users: null,
  });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchStats = async () => {
      // List endpoints now return an envelope; a 1-item page is enough to
      // read `total`. Machines still returns a bare array. allSettled lets
      // each card render whatever succeeded.
      const requests = [
        apiClient.get('/api/proposals', { params: { per_page: 1 } }),
        apiClient.get('/api/rules', { params: { per_page: 1 } }),
        apiClient.get('/api/machines'),
      ];
      if (isAdmin()) {
        requests.push(apiClient.get('/api/users', { params: { per_page: 1 } }));
      }

      const [proposalsRes, rulesRes, machinesRes, usersRes] =
        await Promise.allSettled(requests);

      const envelopeTotal = (result) =>
        result?.status === 'fulfilled' ? result.value.data.total : null;

      setStats({
        proposals: envelopeTotal(proposalsRes),
        rules: envelopeTotal(rulesRes),
        machines:
          machinesRes.status === 'fulfilled' && Array.isArray(machinesRes.value.data)
            ? machinesRes.value.data.length
            : null,
        users: envelopeTotal(usersRes),
      });
      setLoading(false);
    };

    fetchStats();
  }, [isAdmin]);

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  const contrast = (bg) => theme.palette.getContrastText(bg);

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Welcome, {user?.username}!
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 4 }}>
        Santa Sync Server Dashboard
      </Typography>

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <StatCard
            title="Proposals"
            value={stats.proposals}
            icon={<VoteIcon />}
            color={statColors.blue}
            contrast={contrast(statColors.blue)}
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <StatCard
            title="Active Rules"
            value={stats.rules}
            icon={<RuleIcon />}
            color={statColors.green}
            contrast={contrast(statColors.green)}
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <StatCard
            title="Machines"
            value={stats.machines}
            icon={<ComputerIcon />}
            color={statColors.orange}
            contrast={contrast(statColors.orange)}
          />
        </Grid>
        {isAdmin() && (
          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
            <StatCard
              title="Users"
              value={stats.users}
              icon={<PeopleIcon />}
              color={statColors.purple}
              contrast={contrast(statColors.purple)}
            />
          </Grid>
        )}
      </Grid>
    </Box>
  );
};

export default Dashboard;
