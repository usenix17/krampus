import { Box, Typography } from '@mui/material';

const PageHeader = ({ title, subtitle, actions }) => (
  <Box
    sx={{
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
      mb: 3,
    }}
  >
    <Box>
      <Typography variant="h4">{title}</Typography>
      {subtitle && (
        <Typography variant="body2" color="text.secondary">
          {subtitle}
        </Typography>
      )}
    </Box>
    {actions && <Box>{actions}</Box>}
  </Box>
);

export default PageHeader;
