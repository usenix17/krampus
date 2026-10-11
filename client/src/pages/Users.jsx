import { useState } from 'react';
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
import { Refresh as RefreshIcon, Edit as EditIcon, Delete as DeleteIcon } from '@mui/icons-material';
import apiClient from '../api/client';
import { useToast } from '../contexts/ToastContext';
import usePagedQuery from '../hooks/usePagedQuery';
import PageHeader from '../components/PageHeader';
import DataTable from '../components/DataTable';

const Users = () => {
  const toast = useToast();
  const {
    items: users,
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
  } = usePagedQuery('/api/users');
  const [openEdit, setOpenEdit] = useState(false);
  const [selectedUser, setSelectedUser] = useState(null);
  const [newRole, setNewRole] = useState('');

  const handleEditRole = async () => {
    try {
      await apiClient.put(`/api/users/${selectedUser.id}`, { role: newRole });
      toast.success('User role updated successfully!');
      setOpenEdit(false);
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to update user role');
    }
  };

  const handleDelete = async (user) => {
    if (!window.confirm(`Are you sure you want to delete ${user.username}?`)) return;
    try {
      await apiClient.delete(`/api/users/${user.id}`);
      toast.success('User deleted successfully!');
      refresh();
    } catch (err) {
      toast.error(err.response?.data?.error || 'Failed to delete user');
    }
  };

  const columns = [
    { key: 'username', label: 'Username' },
    { key: 'email', label: 'Email', render: (row) => row.email || '-' },
    {
      key: 'role',
      label: 'Role',
      render: (row) => (
        <Chip
          label={row.role}
          size="small"
          color={row.role === 'ADMIN' ? 'primary' : 'default'}
        />
      ),
    },
    {
      key: 'created_at',
      label: 'Created',
      render: (row) => new Date(row.created_at).toLocaleString(),
    },
    {
      key: 'last_login',
      label: 'Last Login',
      render: (row) => (row.last_login ? new Date(row.last_login).toLocaleString() : '-'),
    },
    {
      key: 'actions',
      label: 'Actions',
      align: 'right',
      render: (row) => (
        <>
          <IconButton
            aria-label="edit user role"
            size="small"
            onClick={() => {
              setSelectedUser(row);
              setNewRole(row.role);
              setOpenEdit(true);
            }}
          >
            <EditIcon />
          </IconButton>
          <IconButton
            aria-label="delete user"
            color="error"
            size="small"
            onClick={() => handleDelete(row)}
          >
            <DeleteIcon />
          </IconButton>
        </>
      ),
    },
  ];

  return (
    <Box>
      <PageHeader
        title="Users"
        actions={
          <IconButton aria-label="refresh users" onClick={refresh}>
            <RefreshIcon />
          </IconButton>
        }
      />

      <DataTable
        columns={columns}
        rows={users}
        total={total}
        page={page}
        perPage={perPage}
        onPageChange={setPage}
        onPerPageChange={setPerPage}
        loading={loading}
        error={error}
        onRetry={refresh}
        emptyMessage="No users found"
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Search users..."
      />

      <Dialog open={openEdit} onClose={() => setOpenEdit(false)}>
        <DialogTitle>Edit User Role</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 2 }}>
            User: {selectedUser?.username}
          </Typography>
          <TextField
            fullWidth
            select
            label="Role"
            value={newRole}
            onChange={(e) => setNewRole(e.target.value)}
            margin="normal"
          >
            <MenuItem value="USER">User</MenuItem>
            <MenuItem value="ADMIN">Admin</MenuItem>
          </TextField>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenEdit(false)}>Cancel</Button>
          <Button onClick={handleEditRole} variant="contained">
            Update
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Users;
