import {
  Box,
  Link,
  Card,
  CardActionArea,
  CardContent,
  Grid,
  Typography,
} from '@material-ui/core';
import LaunchIcon from '@material-ui/icons/Launch';
import { useStyles } from './styles';

export const QuickActionsSection: React.FC = () => {
  const classes = useStyles();
  const quickActions = [
    {
      title: 'Create Component',
      description: 'Start a new service',
      link: '/create/templates/default/create-openchoreo-component',
    },
    {
      title: 'View My Projects',
      description: 'Manage your work',
      link: '/catalog?filters[kind]=System&filters[user]=owned',
    },
    {
      title: 'View My Components',
      description: 'View your components',
      link: '/catalog?filters[kind]=Component&filters[user]=owned',
    },
    {
      title: 'Browse Templates',
      description: 'Available Golden Paths',
      link: '/create',
    },
  ];

  return (
    <Box className={classes.overviewSection}>
      <Typography variant="h3">Quick Actions</Typography>
      <Grid container spacing={2} className={classes.quickActionsContainer}>
        {quickActions.map((action, index) => (
          <Grid item xs={12} sm={6} md={6} key={index}>
            <Card className={classes.quickActionCard}>
              <CardActionArea
                className={classes.quickActionCardAction}
                component={Link}
                href={action.link}
                disableRipple
              >
                <CardContent className={classes.quickActionCardContent}>
                  <Box className={classes.quickActionHeader}>
                    <Typography
                      variant="h5"
                      className={classes.quickActionTitle}
                    >
                      {action.title}
                    </Typography>
                    <LaunchIcon
                      fontSize="small"
                      className={classes.quickActionIcon}
                    />
                  </Box>
                  <Typography variant="body2" color="textSecondary">
                    {action.description}
                  </Typography>
                </CardContent>
              </CardActionArea>
            </Card>
          </Grid>
        ))}
      </Grid>
    </Box>
  );
};
