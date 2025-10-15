import { Box, makeStyles } from '@material-ui/core';
import { Typography } from '@material-ui/core';
import { OpenChoreoIcon } from '@openchoreo/backstage-design-system';

const useStyles = makeStyles(theme => ({
  logoText: {
    color: theme.palette.grey[800],
  },
}));

const LogoFull = () => {
  const classes = useStyles();

  return (
    <Box display="flex" alignItems="center" gridGap={8}>
      <OpenChoreoIcon />
      <Typography variant="h3" className={classes.logoText} color="secondary">
        OpenChoreo
      </Typography>
    </Box>
  );
};

export default LogoFull;
