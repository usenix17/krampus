import { useEffect, useState } from 'react';
import { Box, Button, Container, Paper, Typography, Alert, Chip } from '@mui/material';
import { useNavigate, useSearchParams } from 'react-router-dom';
import SecurityIcon from '@mui/icons-material/Security';
import HowToVoteIcon from '@mui/icons-material/HowToVote';
import ComputerIcon from '@mui/icons-material/Computer';
import RuleIcon from '@mui/icons-material/Rule';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { useAuth } from '../contexts/AuthContext';
import apiClient from '../api/client';

const Login = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { user, login } = useAuth();
  const [error, setError] = useState('');

  useEffect(() => {
    if (user) {
      navigate('/dashboard');
      return;
    }

    // Check for auth=success parameter (from OIDC callback)
    const authSuccess = searchParams.get('auth');
    if (authSuccess === 'success') {
      // The token is in an httpOnly cookie, so we can't read it from JavaScript
      // Instead, call /auth/me which will use the cookie automatically
      apiClient.get('/auth/me')
        .then(response => {
          // Store a dummy token since we're using httpOnly cookies
          login('httponly', response.data);
          navigate('/dashboard');
        })
        .catch(err => {
          setError('Failed to fetch user data');
          console.error(err);
        });
    }
  }, [user, searchParams, navigate, login]);

  const handleLogin = () => {
    // Redirect to OIDC login
    window.location.href = '/auth/login';
  };

  const features = [
    { icon: <HowToVoteIcon />, text: 'Vote on binary proposals' },
    { icon: <RuleIcon />, text: 'Manage Santa rules' },
    { icon: <ComputerIcon />, text: 'Register machines and deploy configurations' },
    { icon: <VisibilityIcon />, text: 'View execution events' },
  ];

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
        position: 'relative',
        overflow: 'hidden',
        '&::before': {
          content: '""',
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'radial-gradient(circle at 20% 50%, rgba(255,255,255,0.1) 0%, transparent 50%)',
        },
        '&::after': {
          content: '""',
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'radial-gradient(circle at 80% 50%, rgba(255,255,255,0.1) 0%, transparent 50%)',
        },
      }}
    >
      <Container maxWidth="sm">
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            position: 'relative',
            zIndex: 1,
          }}
        >
          {/* Logo/Icon */}
          <Box
            sx={{
              mb: 3,
              p: 2,
              borderRadius: '50%',
              bgcolor: 'rgba(255, 255, 255, 0.95)',
              boxShadow: '0 8px 32px rgba(0, 0, 0, 0.1)',
            }}
          >
            <SecurityIcon sx={{ fontSize: 60, color: '#667eea' }} />
          </Box>

          {/* Login Card */}
          <Paper
            elevation={8}
            sx={{
              p: 4,
              width: '100%',
              borderRadius: 3,
              bgcolor: 'rgba(255, 255, 255, 0.95)',
              backdropFilter: 'blur(10px)',
            }}
          >
            <Typography component="h1" variant="h4" align="center" gutterBottom fontWeight="bold">
              Krampus Santa Sync
            </Typography>
            <Typography variant="body1" align="center" color="text.secondary" sx={{ mb: 3 }}>
              Collaborative binary allowlist/blocklist management
            </Typography>

            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}

            <Button
              fullWidth
              variant="contained"
              size="large"
              onClick={handleLogin}
              sx={{
                mt: 2,
                py: 1.5,
                background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
                '&:hover': {
                  background: 'linear-gradient(135deg, #5568d3 0%, #63428a 100%)',
                },
                boxShadow: '0 4px 12px rgba(102, 126, 234, 0.4)',
              }}
            >
              Sign in with OIDC
            </Button>

            <Box sx={{ mt: 4 }}>
              <Typography variant="subtitle2" color="text.secondary" fontWeight="bold" sx={{ mb: 2 }}>
                Platform Features
              </Typography>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                {features.map((feature, index) => (
                  <Chip
                    key={index}
                    icon={feature.icon}
                    label={feature.text}
                    variant="outlined"
                    sx={{
                      justifyContent: 'flex-start',
                      py: 2.5,
                      px: 1,
                      '& .MuiChip-icon': {
                        color: '#667eea',
                        ml: 1,
                      },
                    }}
                  />
                ))}
              </Box>
            </Box>
          </Paper>

          {/* Footer */}
          <Typography
            variant="caption"
            sx={{
              mt: 3,
              color: 'rgba(255, 255, 255, 0.9)',
              textAlign: 'center',
            }}
          >
            Collaborative endpoint security management
          </Typography>
        </Box>
      </Container>
    </Box>
  );
};

export default Login;
