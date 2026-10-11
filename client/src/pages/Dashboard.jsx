import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Chip,
  Grid,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Skeleton,
  Typography,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { BarChart } from '@mui/x-charts/BarChart';
import {
  HowToVote as VoteIcon,
  Rule as RuleIcon,
  Computer as ComputerIcon,
  Assessment as EventsIcon,
  CheckCircle as CheckIcon,
  WarningAmber as WarningIcon,
} from '@mui/icons-material';
import apiClient from '../api/client';
import { useAuth } from '../contexts/AuthContext';
import { formatDateTime } from '../utils/datetime';

const TREND_DAYS = 30;

// Short tick label like "3/9"; built from the ISO parts to avoid any
// timezone drift off the UTC-bucketed backend dates.
const shortDate = (iso) => {
  const [, month, day] = iso.split('-');
  return `${Number(month)}/${Number(day)}`;
};

// Full tooltip label like "Mon, Mar 9".
const fullDate = (iso) => {
  const dt = new Date(`${iso}T00:00:00Z`);
  return dt.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  });
};

const formatSync = (value) =>
  value ? `Last sync ${formatDateTime(value)}` : 'Never synced';

// Fill every day in the 30-day window so the axis is continuous even on
// days the backend omitted (it only returns days that had events).
const buildTrend = (daily) => {
  const byDate = new Map((daily || []).map((d) => [d.date, d]));
  const today = new Date();
  const out = [];
  for (let i = TREND_DAYS - 1; i >= 0; i -= 1) {
    const dt = new Date(today);
    dt.setUTCDate(today.getUTCDate() - i);
    const key = dt.toISOString().slice(0, 10);
    const found = byDate.get(key);
    out.push({
      date: key,
      allowed: found ? found.allowed : 0,
      blocked: found ? found.blocked : 0,
    });
  }
  return out;
};

const StatCard = ({ title, value, icon, color, contrast, loading }) => (
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
      <Typography variant="h3">
        {loading ? (
          <Skeleton width={90} />
        ) : (
          (value !== null && value !== undefined ? value : '...')
        )}
      </Typography>
    </CardContent>
  </Card>
);

const Dashboard = () => {
  const { user } = useAuth();
  const theme = useTheme();
  const { stats: statColors } = theme.palette;
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const fetchOverview = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const res = await apiClient.get('/api/stats/overview');
      setData(res.data);
    } catch {
      setError(true);
      setData(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchOverview();
  }, [fetchOverview]);

  const contrast = (bg) => theme.palette.getContrastText(bg);
  const totals = data?.totals || {};
  const trend = buildTrend(data?.daily);
  const hasTrend = trend.some((d) => d.allowed > 0 || d.blocked > 0);
  const topBlocked = data?.top_blocked || [];
  const staleMachines = data?.stale_machines || [];
  const staleAfterHours = data?.stale_after_hours;

  const cards = [
    { title: 'Pending Proposals', value: totals.pending_proposals, icon: <VoteIcon />, color: statColors.blue },
    { title: 'Active Rules', value: totals.rules, icon: <RuleIcon />, color: statColors.green },
    { title: 'Machines', value: totals.machines, icon: <ComputerIcon />, color: statColors.orange },
    { title: 'Events', value: totals.events, icon: <EventsIcon />, color: statColors.purple },
  ];

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Welcome, {user?.username}!
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 4 }}>
        Santa Sync Server Dashboard
      </Typography>

      {error && (
        <Alert
          severity="error"
          sx={{ mb: 3 }}
          action={
            <Button color="inherit" size="small" onClick={fetchOverview}>
              Retry
            </Button>
          }
        >
          Failed to load dashboard statistics.
        </Alert>
      )}

      <Grid container spacing={3}>
        {cards.map((card) => (
          <Grid key={card.title} size={{ xs: 12, sm: 6, md: 3 }}>
            <StatCard
              title={card.title}
              value={card.value}
              icon={card.icon}
              color={card.color}
              contrast={contrast(card.color)}
              loading={loading}
            />
          </Grid>
        ))}
      </Grid>

      {!error && (
        <Grid container spacing={3} sx={{ mt: 1 }}>
          <Grid size={{ xs: 12, md: 8 }}>
            <Card>
              <CardHeader
                title="Allow / block activity"
                subheader="Last 30 days"
              />
              <CardContent>
                {loading ? (
                  <Skeleton variant="rectangular" height={300} />
                ) : hasTrend ? (
                  <BarChart
                    dataset={trend}
                    height={300}
                    borderRadius={4}
                    grid={{ horizontal: true }}
                    xAxis={[
                      {
                        scaleType: 'band',
                        dataKey: 'date',
                        label: 'Date',
                        valueFormatter: (value, ctx) =>
                          ctx.location === 'tick' ? shortDate(value) : fullDate(value),
                        tickLabelInterval: (value, index) => index % 5 === 0,
                      },
                    ]}
                    yAxis={[{ label: 'Executions' }]}
                    series={[
                      {
                        dataKey: 'allowed',
                        label: 'Allowed',
                        color: theme.palette.success.main,
                        stack: 'total',
                      },
                      {
                        dataKey: 'blocked',
                        label: 'Blocked',
                        color: theme.palette.error.main,
                        stack: 'total',
                      },
                    ]}
                  />
                ) : (
                  <Box sx={{ py: 6, textAlign: 'center' }}>
                    <Typography color="text.secondary">
                      No execution events in the last 30 days
                    </Typography>
                  </Box>
                )}
              </CardContent>
            </Card>
          </Grid>

          <Grid size={{ xs: 12, md: 4 }}>
            <Card sx={{ height: '100%' }}>
              <CardHeader title="Top blocked binaries" />
              <CardContent sx={{ pt: 0 }}>
                {loading ? (
                  <Skeleton variant="rectangular" height={260} />
                ) : topBlocked.length === 0 ? (
                  <Typography color="text.secondary" sx={{ py: 2 }}>
                    No blocked executions
                  </Typography>
                ) : (
                  <List dense disablePadding>
                    {topBlocked.map((item) => (
                      <ListItem
                        key={item.file_hash}
                        disableGutters
                        secondaryAction={
                          <Chip
                            label={item.count}
                            size="small"
                            color="error"
                            variant="outlined"
                          />
                        }
                      >
                        <ListItemText
                          primary={item.bundle_name || item.file_hash.slice(0, 12)}
                          secondary={item.file_path || undefined}
                          slotProps={{
                            primary: { title: item.file_hash, noWrap: true },
                            secondary: { noWrap: true },
                          }}
                          sx={{ pr: 6 }}
                        />
                      </ListItem>
                    ))}
                  </List>
                )}
              </CardContent>
            </Card>
          </Grid>

          <Grid size={{ xs: 12 }}>
            <Card>
              <CardHeader
                title="Sync health"
                subheader={
                  staleAfterHours != null
                    ? `Stale after ${staleAfterHours}h without a sync`
                    : undefined
                }
              />
              <CardContent sx={{ pt: 0 }}>
                {loading ? (
                  <Skeleton variant="rectangular" height={80} />
                ) : staleMachines.length === 0 ? (
                  <List dense disablePadding>
                    <ListItem disableGutters>
                      <ListItemIcon sx={{ minWidth: 40 }}>
                        <CheckIcon color="success" />
                      </ListItemIcon>
                      <ListItemText primary="All machines syncing" />
                    </ListItem>
                  </List>
                ) : (
                  <List dense disablePadding>
                    {staleMachines.map((machine) => (
                      <ListItem key={machine.machine_id} disableGutters>
                        <ListItemIcon sx={{ minWidth: 40 }}>
                          <WarningIcon color="warning" />
                        </ListItemIcon>
                        <ListItemText
                          primary={machine.hostname || machine.machine_id}
                          secondary={formatSync(machine.last_sync)}
                        />
                      </ListItem>
                    ))}
                  </List>
                )}
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      )}
    </Box>
  );
};

export default Dashboard;
