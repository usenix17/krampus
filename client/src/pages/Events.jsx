import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  Chip,
  TablePagination,
  TextField,
  MenuItem,
  IconButton,
  Tooltip,
} from '@mui/material';
import AddCircleIcon from '@mui/icons-material/AddCircle';
import { getEvents } from '../api/events';

const Events = () => {
  const navigate = useNavigate();
  const [events, setEvents] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(50);
  const [filter, setFilter] = useState('');

  const loadEvents = async () => {
    try {
      const response = await getEvents({
        page: page + 1,
        limit: rowsPerPage,
        decision: filter,
      });
      setEvents(response.data.events || []);
      setTotal(response.data.total || 0);
    } catch (error) {
      console.error('Failed to load events:', error);
    }
  };

  useEffect(() => {
    loadEvents();
  }, [page, rowsPerPage, filter]);

  const handleChangePage = (event, newPage) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  const getDecisionColor = (decision) => {
    switch (decision) {
      case 'ALLOW':
        return 'success';
      case 'BLOCK':
        return 'error';
      default:
        return 'default';
    }
  };

  const handleCreateRule = (event) => {
    // Navigate to proposals page with hash and bundle info pre-filled
    const params = new URLSearchParams({
      hash: event.file_hash
    });

    // Add bundle name or bundle ID as comment if available
    if (event.bundle_name) {
      params.append('comment', event.bundle_name);
    } else if (event.bundle_id) {
      params.append('comment', event.bundle_id);
    }

    navigate(`/proposals?${params.toString()}`);
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4">Execution Events</Typography>
        <TextField
          select
          label="Filter by Decision"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          sx={{ minWidth: 150 }}
          size="small"
        >
          <MenuItem value="">All</MenuItem>
          <MenuItem value="ALLOW">Allow</MenuItem>
          <MenuItem value="BLOCK">Block</MenuItem>
        </TextField>
      </Box>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Timestamp</TableCell>
              <TableCell>Machine</TableCell>
              <TableCell>File Path</TableCell>
              <TableCell>SHA256 Hash</TableCell>
              <TableCell>Decision</TableCell>
              <TableCell>User</TableCell>
              <TableCell>Bundle</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {events.map((event) => (
              <TableRow key={event.id}>
                <TableCell>
                  {new Date(event.execution_time).toLocaleString()}
                </TableCell>
                <TableCell>{event.machine_id}</TableCell>
                <TableCell>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                    {event.file_path}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Typography
                    variant="body2"
                    sx={{
                      fontFamily: 'monospace',
                      fontSize: '0.7rem',
                      wordBreak: 'break-all',
                      maxWidth: '400px'
                    }}
                  >
                    {event.file_hash}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Chip
                    label={event.decision}
                    color={getDecisionColor(event.decision)}
                    size="small"
                  />
                </TableCell>
                <TableCell>{event.executing_user}</TableCell>
                <TableCell>
                  {event.bundle_name && (
                    <>
                      <Typography variant="body2">{event.bundle_name}</Typography>
                      <Typography variant="caption" color="text.secondary">
                        {event.bundle_id}
                      </Typography>
                    </>
                  )}
                </TableCell>
                <TableCell>
                  <Tooltip title="Create rule for this binary">
                    <IconButton
                      size="small"
                      color="primary"
                      onClick={() => handleCreateRule(event)}
                    >
                      <AddCircleIcon />
                    </IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <TablePagination
          rowsPerPageOptions={[25, 50, 100]}
          component="div"
          count={total}
          rowsPerPage={rowsPerPage}
          page={page}
          onPageChange={handleChangePage}
          onRowsPerPageChange={handleChangeRowsPerPage}
        />
      </TableContainer>
    </Box>
  );
};

export default Events;
