import {
  Alert,
  Box,
  Button,
  InputAdornment,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
} from '@mui/material';
import { Search as SearchIcon } from '@mui/icons-material';

// Shared server-side paginated table. Page numbers from the hook are 1-based;
// MUI TablePagination is 0-based, so convert at the boundary.
const DataTable = ({
  columns,
  rows,
  total,
  page,
  perPage,
  onPageChange,
  onPerPageChange,
  loading,
  error,
  onRetry,
  emptyMessage = 'No records found',
  search,
  onSearchChange,
  searchPlaceholder = 'Search...',
  toolbarActions,
}) => {
  const colSpan = columns.length;
  // Hide the search field entirely when the caller omits a search handler, so
  // endpoints without search do not render an inert input.
  const showSearch = typeof onSearchChange === 'function';

  return (
    <Box>
      {(showSearch || toolbarActions) && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: 2,
            mb: 2,
            flexWrap: 'wrap',
          }}
        >
          {showSearch ? (
            <TextField
              size="small"
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={searchPlaceholder}
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon fontSize="small" />
                    </InputAdornment>
                  ),
                },
                htmlInput: { 'aria-label': searchPlaceholder || 'search table' },
              }}
              sx={{ minWidth: 240 }}
            />
          ) : (
            <Box />
          )}
          {toolbarActions && <Box>{toolbarActions}</Box>}
        </Box>
      )}

      {error && (
        <Alert
          severity="error"
          sx={{ mb: 2 }}
          action={
            onRetry && (
              <Button color="inherit" size="small" onClick={onRetry}>
                Retry
              </Button>
            )
          }
        >
          {error}
        </Alert>
      )}

      <TableContainer sx={{ overflowX: 'auto' }}>
        <Table aria-label="data table">
          <TableHead>
            <TableRow>
              {columns.map((col) => (
                <TableCell key={col.key} align={col.align || 'left'}>
                  {col.label}
                </TableCell>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {loading &&
              [0, 1, 2].map((r) => (
                <TableRow key={`skeleton-${r}`}>
                  {columns.map((col) => (
                    <TableCell key={col.key} align={col.align || 'left'}>
                      <Skeleton variant="text" />
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            {!loading && !error && rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={colSpan} align="center" sx={{ py: 4 }}>
                  {emptyMessage}
                </TableCell>
              </TableRow>
            )}
            {!loading &&
              rows.map((row, index) => (
                <TableRow key={row.id ?? index} hover>
                  {columns.map((col) => (
                    <TableCell key={col.key} align={col.align || 'left'}>
                      {col.render ? col.render(row) : row[col.key]}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
          </TableBody>
        </Table>
      </TableContainer>

      <TablePagination
        component="div"
        count={total}
        page={total > 0 ? page - 1 : 0}
        rowsPerPage={perPage}
        onPageChange={(event, newPage) => onPageChange(newPage + 1)}
        onRowsPerPageChange={(event) =>
          onPerPageChange(parseInt(event.target.value, 10))
        }
        rowsPerPageOptions={[10, 25, 50, 100]}
      />
    </Box>
  );
};

export default DataTable;
